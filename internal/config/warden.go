package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/rileydj78/palsforge-warden/internal/broadcast"
	"github.com/rileydj78/palsforge-warden/internal/events"
	"github.com/rileydj78/palsforge-warden/internal/gamesettings"
	"github.com/rileydj78/palsforge-warden/internal/notify"
	"github.com/rileydj78/palsforge-warden/internal/paldefender"
)

// Warden is Warden's own configuration, kept in warden.json in its data folder. Everything has a
// sensible default, so a new install only needs the server folder.
type Warden struct {
	ServerDir string             `json:"serverDir"`
	Control   Control            `json:"control"`
	Nightly   Nightly            `json:"nightly"`
	Backups   Backups            `json:"backups"`
	Update    Update             `json:"update"`
	Notify    notify.Config      `json:"notify"`
	PalDef    paldefender.Config `json:"palDefender"`
	Broadcast broadcast.Config   `json:"broadcasts"`
	Health    Health             `json:"health"`
	// Weekly events, one per line: "Name | Fri 18:00 | Sun 22:00 | ExpRate=2, PalCaptureRate=1.5".
	Events string `json:"events"`
}

// Health is what Warden does on its own to keep the server up.
type Health struct {
	// Restart a server that's running but hasn't answered for this many minutes (0 = only alert).
	RestartFrozenMinutes int `json:"restartFrozenMinutes"`
	// Restart (with in-game warnings) when the server uses more memory than this (0 = off).
	MemoryLimitGB float64 `json:"memoryLimitGB"`
}

// Control says how Warden starts and stops the game server.
type Control struct {
	// "supervise": Warden runs the server itself and restarts it if it crashes (the default).
	// "external": something else runs it (a service, a scheduled task, a host's panel); Warden
	// stops it through the REST API and starts it with StartCommand.
	Mode string   `json:"mode"`
	Exe  string   `json:"exe"`  // supervise: the server program, relative to ServerDir
	Args []string `json:"args"` // supervise: its arguments
	// supervise, Windows: the CPU cores it may use, e.g. "8-11" or "0,2,4"; empty = all.
	Affinity string `json:"affinity"`
	// external: the command that starts the server, e.g. ["schtasks", "/Run", "/TN", "Palworld"].
	StartCommand []string `json:"startCommand"`
	// external: a file to create before a planned stop and remove before starting, for watchdog
	// scripts that restart the server unless such a file exists. Relative to ServerDir.
	StopMarker string `json:"stopMarker"`
}

// Nightly is the routine: warn, save, stop, back up, update, start.
type Nightly struct {
	Enabled     bool   `json:"enabled"`
	Time        string `json:"time"`        // "04:00", local time of this machine
	WarnMinutes []int  `json:"warnMinutes"` // in-game warnings before stopping
}

// Backups says where backups go and how many to keep.
type Backups struct {
	Dir     string   `json:"dir"`     // local backups; default <data>/backups
	Offsite []string `json:"offsite"` // second places (folders or network shares), each checked
	Image   string   `json:"image"`   // whole-server image: "daily", "weekly" (Sundays) or "off"
	Daily   int      `json:"keepDaily"`
	Weekly  int      `json:"keepWeekly"`
	Tag     string   `json:"tag"` // file name prefix, e.g. "pal"
}

// Update is the game update through SteamCMD during the nightly routine.
type Update struct {
	Enabled  bool   `json:"enabled"`
	SteamCMD string `json:"steamcmd"` // path to steamcmd(.exe)
	Validate bool   `json:"validate"`
}

// AppID is Palworld Dedicated Server's Steam app.
const AppID = "2394010"

// Defaults returns a configuration for a server folder.
func Defaults(serverDir string) Warden {
	w := Warden{ServerDir: serverDir}
	w.Control.Mode = "supervise"
	if runtime.GOOS == "windows" {
		w.Control.Exe = filepath.Join("Pal", "Binaries", "Win64", "PalServer-Win64-Shipping-Cmd.exe")
		w.Control.Args = []string{"Pal", "-useperfthreads", "-NoAsyncLoadingThread", "-UseMultithreadForDS"}
	} else {
		w.Control.Exe = filepath.Join("Pal", "Binaries", "Linux", "PalServer-Linux-Shipping")
		w.Control.Args = []string{"Pal", "-useperfthreads", "-NoAsyncLoadingThread", "-UseMultithreadForDS"}
	}
	w.Nightly = Nightly{Enabled: true, Time: "04:00", WarnMinutes: []int{15, 5, 1}}
	w.Backups = Backups{Image: "weekly", Daily: 14, Weekly: 8, Tag: "pal"}
	w.Update = Update{Enabled: true}
	w.Health = Health{RestartFrozenMinutes: 5}
	return w
}

var (
	timeRe     = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	tagRe      = regexp.MustCompile(`^[a-z0-9]{1,20}$`)
	affinityRe = regexp.MustCompile(`^\d+(-\d+)?(,\d+(-\d+)?)*$`)
)

// Check reports what's wrong with a configuration, in words an admin can act on.
func (w *Warden) Check() error {
	var bad []string
	if w.ServerDir == "" {
		bad = append(bad, "the server folder isn't set")
	} else if _, err := os.Stat(w.ServerDir); err != nil {
		bad = append(bad, fmt.Sprintf("the server folder %s doesn't exist", w.ServerDir))
	}
	switch w.Control.Mode {
	case "supervise":
		if w.Control.Exe == "" {
			bad = append(bad, "control.exe (the server program) isn't set")
		}
	case "external":
		if len(w.Control.StartCommand) == 0 {
			bad = append(bad, "control.startCommand is needed when something else runs the server")
		}
	default:
		bad = append(bad, `control.mode must be "supervise" or "external"`)
	}
	if w.Control.Affinity != "" && !affinityRe.MatchString(w.Control.Affinity) {
		bad = append(bad, `control.affinity looks like "8-11" or "0,2,4"`)
	}
	if !timeRe.MatchString(w.Nightly.Time) {
		bad = append(bad, `nightly.time must look like "04:00"`)
	}
	for _, m := range w.Nightly.WarnMinutes {
		if m < 1 || m > 60 {
			bad = append(bad, "nightly.warnMinutes must be between 1 and 60")
			break
		}
	}
	if w.Backups.Image != "daily" && w.Backups.Image != "weekly" && w.Backups.Image != "off" {
		bad = append(bad, `backups.image must be "daily", "weekly" or "off"`)
	}
	if w.Backups.Daily < 1 || w.Backups.Weekly < 0 {
		bad = append(bad, "keep at least one daily backup")
	}
	if !tagRe.MatchString(w.Backups.Tag) {
		bad = append(bad, "backups.tag must be 1-20 lowercase letters or digits")
	}
	if w.Update.Enabled && w.Update.SteamCMD != "" {
		if _, err := os.Stat(w.Update.SteamCMD); err != nil {
			bad = append(bad, fmt.Sprintf("SteamCMD isn't at %s", w.Update.SteamCMD))
		}
	}
	if w.Broadcast.EveryMinutes != 0 && (w.Broadcast.EveryMinutes < 5 || w.Broadcast.EveryMinutes > 720) {
		bad = append(bad, "broadcasts.everyMinutes must be between 5 and 720")
	}
	if w.Health.RestartFrozenMinutes < 0 || w.Health.RestartFrozenMinutes > 60 {
		bad = append(bad, "health.restartFrozenMinutes must be 0 (off) to 60")
	}
	if w.Health.MemoryLimitGB < 0 || (w.Health.MemoryLimitGB > 0 && w.Health.MemoryLimitGB < 4) {
		bad = append(bad, "health.memoryLimitGB must be 0 (off) or at least 4")
	}
	if evs, err := events.Parse(w.Events); err != nil {
		bad = append(bad, "events: "+err.Error())
	} else {
		for _, e := range evs {
			for k, v := range e.Settings {
				if err := gamesettings.ValidateKnown(k, v); err != nil {
					bad = append(bad, "event "+e.Name+": "+err.Error())
				}
			}
		}
	}
	if err := w.Notify.Check(); err != nil {
		bad = append(bad, err.Error())
	}
	if len(bad) > 0 {
		return errors.New(strings.Join(bad, "; "))
	}
	return nil
}

// ConfigPath is warden.json in the data folder.
func ConfigPath(dataDir string) string { return filepath.Join(dataDir, "warden.json") }

// Load reads warden.json, filling anything missing from the defaults. A missing file is not an
// error: it returns the defaults for serverDir (the caller saves them so the admin can edit them).
func Load(dataDir, serverDir string) (Warden, bool, error) {
	w := Defaults(serverDir)
	b, err := os.ReadFile(ConfigPath(dataDir))
	if errors.Is(err, fs.ErrNotExist) {
		return w, false, nil
	}
	if err != nil {
		return w, false, err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")) // Notepad and PowerShell add a byte-order mark
	if err := json.Unmarshal(b, &w); err != nil {
		return w, true, fmt.Errorf("warden.json isn't valid: %w", err)
	}
	if serverDir != "" {
		w.ServerDir = serverDir // the command line wins
	}
	return w, true, nil
}

// Save writes warden.json atomically (temp file, then rename).
func Save(dataDir string, w Warden) error {
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	tmp := ConfigPath(dataDir) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, ConfigPath(dataDir))
}

// Abs resolves a path relative to the server folder.
func (w *Warden) Abs(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(w.ServerDir, p)
}
