// Package host starts, stops, watches and updates the game server on this machine.
//
// Two modes (config.Control.Mode):
//   - supervise: Warden runs the server itself, restarts it when it crashes, and keeps it stopped
//     when an admin or the nightly routine stops it on purpose.
//   - external: something else runs the server. Warden starts it with a configured command and,
//     before a planned stop, creates a marker file that watchdog scripts can respect.
package host

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/config"
)

// Event is something worth telling the admin (a crash, a restart, giving up).
type Event struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
	Text string    `json:"text"`
}

// Host controls the server process.
type Host struct {
	cfg     config.Warden
	logPath string // the server's own console output (supervise mode)
	OnEvent func(Event)

	mu      sync.Mutex
	want    bool      // supervise: should the server be running?
	started time.Time // when Warden last started it
	crashes []time.Time
	pausedT time.Time // after too many crashes, wait until this time
	cmd     *exec.Cmd
}

// New returns a Host. dataDir is where the server's console log goes in supervise mode.
func New(cfg config.Warden, dataDir string) *Host {
	return &Host{cfg: cfg, logPath: filepath.Join(dataDir, "server-console.log"), want: true}
}

func (h *Host) event(kind, text string) {
	if h.OnEvent != nil {
		h.OnEvent(Event{At: time.Now(), Kind: kind, Text: text})
	}
}

// Running says whether any server process is up.
func (h *Host) Running() bool {
	pids, err := pidsOf(h.cfg.Abs(h.cfg.Control.Exe))
	return err == nil && len(pids) > 0
}

// Status for the panel.
type Status struct {
	Mode       string    `json:"mode"`
	Running    bool      `json:"running"`
	Wanted     bool      `json:"wanted"`
	Started    time.Time `json:"started,omitempty"`
	Crashes24h int       `json:"crashes24h"`
	PausedTill time.Time `json:"pausedTill,omitempty"`
}

// State reports the current status.
func (h *Host) State() Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, c := range h.crashes {
		if time.Since(c) < 24*time.Hour {
			n++
		}
	}
	return Status{Mode: h.cfg.Control.Mode, Running: h.Running(), Wanted: h.want, Started: h.started, Crashes24h: n, PausedTill: h.pausedT}
}

// PlanStop says the next stop is on purpose: supervise mode won't restart the server, and
// external mode creates the stop marker so a watchdog script leaves it stopped.
func (h *Host) PlanStop(ctx context.Context) error {
	h.mu.Lock()
	h.want = false
	h.mu.Unlock()
	if h.cfg.Control.Mode == "external" && h.cfg.Control.StopMarker != "" {
		return os.WriteFile(h.cfg.Abs(h.cfg.Control.StopMarker), []byte("stopped by Warden "+time.Now().Format(time.RFC3339)+"\n"), 0o644)
	}
	return nil
}

// WaitStopped waits up to d for every server process to exit.
func (h *Host) WaitStopped(ctx context.Context, d time.Duration) bool {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if !h.Running() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
	}
	return !h.Running()
}

// ForceStop kills the server processes.
func (h *Host) ForceStop(ctx context.Context) error {
	if err := h.PlanStop(ctx); err != nil {
		return err
	}
	pids, err := pidsOf(h.cfg.Abs(h.cfg.Control.Exe))
	if err != nil {
		return err
	}
	var errs []error
	for _, pid := range pids {
		if err := kill(pid); err != nil {
			errs = append(errs, err)
		}
	}
	h.event("stop", fmt.Sprintf("forced the server to stop (%d process(es))", len(pids)))
	return errors.Join(errs...)
}

// Start starts the server (or lets the supervisor start it).
func (h *Host) Start(ctx context.Context) error {
	h.mu.Lock()
	h.want = true
	h.pausedT = time.Time{}
	h.mu.Unlock()
	if h.cfg.Control.Mode == "external" {
		if h.cfg.Control.StopMarker != "" {
			if err := os.Remove(h.cfg.Abs(h.cfg.Control.StopMarker)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if h.Running() {
			return nil
		}
		c := h.cfg.Control.StartCommand
		cmd := exec.CommandContext(ctx, c[0], c[1:]...)
		hideWindow(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("the start command failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
		h.event("start", "asked the server to start")
		return nil
	}
	if h.Running() {
		return nil
	}
	return h.launch()
}

// launch starts the server program (supervise mode).
func (h *Host) launch() error {
	exe := h.cfg.Abs(h.cfg.Control.Exe)
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("the server program isn't at %s", exe)
	}
	logf, err := os.OpenFile(h.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, h.cfg.Control.Args...)
	cmd.Dir = h.cfg.ServerDir
	cmd.Stdout, cmd.Stderr = logf, logf
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		logf.Close()
		return fmt.Errorf("could not start the server: %w", err)
	}
	if h.cfg.Control.Affinity != "" {
		if mask, err := ParseAffinity(h.cfg.Control.Affinity); err == nil {
			if err := setAffinity(cmd.Process.Pid, mask); err != nil {
				h.event("warning", "couldn't set the CPU cores: "+err.Error())
			}
		}
	}
	h.mu.Lock()
	h.cmd, h.started = cmd, time.Now()
	h.mu.Unlock()
	h.event("start", fmt.Sprintf("started the server (process %d)", cmd.Process.Pid))
	go func() {
		err := cmd.Wait()
		logf.Close()
		h.mu.Lock()
		wanted := h.want
		if h.cmd == cmd {
			h.cmd = nil
		}
		h.mu.Unlock()
		if wanted {
			h.event("crash", fmt.Sprintf("the server stopped on its own (%v)", err))
		}
	}()
	return nil
}

// Supervise keeps the server running in supervise mode until ctx ends. It adopts a server that
// is already running, restarts after a crash (waiting longer after each), and gives up for half
// an hour after five crashes in ten minutes, so a broken update can't loop forever.
func (h *Host) Supervise(ctx context.Context) {
	if h.cfg.Control.Mode != "supervise" {
		return
	}
	wait := 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		h.mu.Lock()
		want, paused := h.want, time.Now().Before(h.pausedT)
		h.mu.Unlock()
		if !want || paused || h.Running() {
			wait = 5 * time.Second
			continue
		}
		h.mu.Lock()
		now := time.Now()
		recent := h.crashes[:0]
		for _, c := range h.crashes {
			if now.Sub(c) < 24*time.Hour {
				recent = append(recent, c)
			}
		}
		h.crashes = append(recent, now)
		n := 0
		for _, c := range h.crashes {
			if now.Sub(c) < 10*time.Minute {
				n++
			}
		}
		if n >= 5 {
			h.pausedT = now.Add(30 * time.Minute)
			h.mu.Unlock()
			h.event("gave-up", "the server keeps stopping (5 times in 10 minutes); waiting 30 minutes before trying again")
			continue
		}
		h.mu.Unlock()
		if err := h.launch(); err != nil {
			h.event("crash", err.Error())
		}
		wait = time.Duration(min(60, 5*n)) * time.Second
	}
}

// ParseAffinity turns "8-11" or "0,2,4" into a CPU bit mask.
func ParseAffinity(s string) (uint64, error) {
	var mask uint64
	for _, part := range strings.Split(s, ",") {
		lo, hi, isRange := strings.Cut(strings.TrimSpace(part), "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			return 0, fmt.Errorf("bad core %q", part)
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil {
				return 0, fmt.Errorf("bad core range %q", part)
			}
		}
		if a < 0 || b > 63 || a > b {
			return 0, fmt.Errorf("cores must be 0-63, low to high: %q", part)
		}
		for i := a; i <= b; i++ {
			mask |= 1 << uint(i)
		}
	}
	return mask, nil
}

// ---- updates through SteamCMD

var buildRe = regexp.MustCompile(`"buildid"\s+"(\d+)"`)

// BuildID reads the installed build from SteamCMD's app manifest ("" if unknown).
func (h *Host) BuildID() string {
	b, err := os.ReadFile(filepath.Join(h.cfg.ServerDir, "steamapps", "appmanifest_"+config.AppID+".acf"))
	if err != nil {
		return ""
	}
	if m := buildRe.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

// Update runs SteamCMD for the server and says what changed.
func (h *Host) Update(ctx context.Context) (string, error) {
	if !h.cfg.Update.Enabled {
		return "updates are off", nil
	}
	if h.cfg.Update.SteamCMD == "" {
		return "", errors.New("SteamCMD's location isn't set (update.steamcmd)")
	}
	before := h.BuildID()
	args := []string{"+force_install_dir", h.cfg.ServerDir, "+login", "anonymous", "+app_update", config.AppID}
	if h.cfg.Update.Validate {
		args = append(args, "validate")
	}
	args = append(args, "+quit")
	ctx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.cfg.Update.SteamCMD, args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	tail := lastLines(string(out), 3)
	ok := strings.Contains(string(out), "Success! App '"+config.AppID+"'") || strings.Contains(string(out), "already up to date")
	if err != nil && !ok {
		return "", fmt.Errorf("SteamCMD failed: %v: %s", err, tail)
	}
	if !ok {
		return "", fmt.Errorf("SteamCMD didn't report success: %s", tail)
	}
	after := h.BuildID()
	switch {
	case before != "" && after != "" && before != after:
		h.event("update", "game updated from build "+before+" to "+after)
		return "updated from build " + before + " to " + after, nil
	case after != "":
		return "already up to date (build " + after + ")", nil
	}
	return "SteamCMD finished", nil
}

func lastLines(s string, n int) string {
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" {
			lines = append(lines, t)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " / ")
}

// ConsoleTail returns the last n lines of the server's console log (supervise mode).
func (h *Host) ConsoleTail(n int) string {
	f, err := os.Open(h.logPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, _ := f.Stat()
	off := st.Size() - 64<<10
	if off < 0 {
		off = 0
	}
	f.Seek(off, io.SeekStart)
	b, _ := io.ReadAll(f)
	return lastLines(string(b), n)
}
