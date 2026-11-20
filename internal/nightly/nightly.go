// Package nightly connects the routine engine to this server: what goes into backups, where the
// copies go, when the routine runs, and the record of every run.
package nightly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/backup"
	"github.com/rileydj78/palsforge-warden/internal/config"
	"github.com/rileydj78/palsforge-warden/internal/gamesettings"
	"github.com/rileydj78/palsforge-warden/internal/host"
	"github.com/rileydj78/palsforge-warden/internal/mods"
	"github.com/rileydj78/palsforge-warden/internal/notify"
	"github.com/rileydj78/palsforge-warden/internal/palworld"
	"github.com/rileydj78/palsforge-warden/internal/routine"
	"github.com/rileydj78/palsforge-warden/internal/store"
)

// DataPaths is what a data backup holds, relative to the server folder: the world and settings,
// and every mod location (missing ones are skipped).
var DataPaths = []string{
	"Pal/Saved",
	"Pal/Content/Paks/~mods",
	"Pal/Binaries/Win64/ue4ss/Mods",
	"Pal/Binaries/Win64/PalDefender",
}

// skipData leaves out the game's own rolling backups (they triple the size and Warden keeps its own)
// and crash dumps.
func skipData(rel string) bool {
	return strings.Contains(rel, "/SaveGames/") && strings.Contains(rel, "/backup/") ||
		strings.Contains(rel, "/Crashes/")
}

// skipImage leaves out what SteamCMD can download again and the data backup already has.
func skipImage(rel string) bool {
	return rel == "steamapps/downloading" || strings.HasPrefix(rel, "steamapps/temp") || skipData(rel)
}

// Backups implements routine.Backups for this server.
type Backups struct {
	Cfg     config.Warden
	DataDir string
	Now     func() time.Time
	made    []*backup.Archive
}

// LocalDir is where backups are written first.
func (b *Backups) LocalDir() string {
	if b.Cfg.Backups.Dir != "" {
		return b.Cfg.Backups.Dir
	}
	return filepath.Join(b.DataDir, "backups")
}

func (b *Backups) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// WantImage says whether tonight's run also makes a whole-server image.
func (b *Backups) WantImage() bool {
	switch b.Cfg.Backups.Image {
	case "daily":
		return true
	case "weekly":
		return b.now().Weekday() == time.Sunday
	}
	return false
}

// MakeAll makes tonight's archives.
func (b *Backups) MakeAll(ctx context.Context) ([]string, error) {
	b.made = nil
	t := b.now()
	a, err := backup.Make(b.LocalDir(), backup.FileName(b.Cfg.Backups.Tag, backup.Data, t), b.Cfg.ServerDir, DataPaths, skipData)
	if err != nil {
		return nil, fmt.Errorf("data backup: %w", err)
	}
	b.made = append(b.made, a)
	names := []string{a.Name}
	if b.WantImage() {
		img, err := backup.Make(b.LocalDir(), backup.FileName(b.Cfg.Backups.Tag, backup.Server, t), b.Cfg.ServerDir, []string{"."}, skipImage)
		if err != nil {
			return names, fmt.Errorf("server image: %w", err)
		}
		b.made = append(b.made, img)
		names = append(names, img.Name)
	}
	return names, nil
}

// CopyAndVerify copies tonight's archives to every second place and checks each copy.
func (b *Backups) CopyAndVerify(ctx context.Context, names []string) error {
	if len(b.Cfg.Backups.SFTP)+len(b.Cfg.Backups.S3) > 0 {
		var errs []error
		for _, t := range b.Cfg.Backups.SFTP {
			for _, a := range b.made {
				if err := t.SendVerified(ctx, *a); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", t.Name(), err))
				}
			}
		}
		for _, t := range b.Cfg.Backups.S3 {
			for _, a := range b.made {
				if err := t.SendVerified(ctx, *a); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", t.Name(), err))
				}
			}
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		if len(b.Cfg.Backups.Offsite) == 0 {
			return nil // the SFTP copies are the second place, each read back and checked
		}
	}
	if len(b.Cfg.Backups.Offsite) == 0 {
		return errors.New("no second place is set for backups (backups.offsite): they exist on this machine only")
	}
	var errs []error
	for _, dst := range b.Cfg.Backups.Offsite {
		for _, a := range b.made {
			if err := backup.CopyVerified(a, dst); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", dst, err))
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	// Prove the copy in the first second place really restores: every file read back, a world in it.
	for _, a := range b.made {
		if a.Kind != backup.Data {
			continue
		}
		p, err := backup.Prove(filepath.Join(b.Cfg.Backups.Offsite[0], a.Name))
		saveProof(b.DataDir, a.Name, p, err)
		if err != nil {
			return fmt.Errorf("the copy of %s doesn't restore: %w", a.Name, err)
		}
	}
	return nil
}

// ProofRecord is the last time a backup was read back completely.
type ProofRecord struct {
	Name  string    `json:"name"`
	At    time.Time `json:"at"`
	OK    bool      `json:"ok"`
	World string    `json:"world,omitempty"`
	Error string    `json:"error,omitempty"`
}

func saveProof(dataDir, name string, p *backup.Proof, err error) {
	r := ProofRecord{Name: name, At: time.Now(), OK: err == nil}
	if p != nil {
		r.World = p.World
	}
	if err != nil {
		r.Error = err.Error()
	}
	b, _ := json.Marshal(r)
	os.WriteFile(filepath.Join(dataDir, "last-proof.json"), b, 0o600)
}

// LastProof returns the last proof, if any.
func LastProof(dataDir string) *ProofRecord {
	b, err := os.ReadFile(filepath.Join(dataDir, "last-proof.json"))
	if err != nil {
		return nil
	}
	var r ProofRecord
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

// PinLatest keeps the newest data backup (here and in every second place) whatever retention
// says, e.g. the last one made before a crash, so routine backups of a broken world can't push it
// out.
func (r *Runner) PinLatest(why string) {
	for _, dir := range append([]string{r.Backups.LocalDir()}, r.Cfg.Backups.Offsite...) {
		list, _ := backup.ListTag(dir, r.Cfg.Backups.Tag)
		for _, a := range list {
			if a.Kind == backup.Data {
				if a.Pinned == "" {
					backup.Pin(dir, a.Name, why)
				}
				break
			}
		}
		// Keep only the newest 3 automatic pins, so a server that crashes often can't fill the disk.
		list, _ = backup.ListTag(dir, r.Cfg.Backups.Tag)
		n := 0
		for _, a := range list {
			if strings.HasPrefix(a.Pinned, "the last backup before") {
				if n++; n > 3 {
					backup.Unpin(dir, a.Name)
				}
			}
		}
	}
}

// Prune applies retention locally and in every second place.
func (b *Backups) Prune(ctx context.Context) error {
	p := backup.Policy{Daily: b.Cfg.Backups.Daily, Weekly: b.Cfg.Backups.Weekly}
	var errs []error
	for _, dir := range append([]string{b.LocalDir()}, b.Cfg.Backups.Offsite...) {
		if _, err := backup.Prune(dir, p, b.Cfg.Backups.Tag); err != nil {
			errs = append(errs, err)
		}
	}
	if len(b.Cfg.Backups.SFTP)+len(b.Cfg.Backups.S3) > 0 { // other machines keep what this one keeps
		keep := map[string]bool{}
		if list, err := backup.ListTag(b.LocalDir(), b.Cfg.Backups.Tag); err == nil {
			for _, a := range list {
				keep[a.Name] = true
			}
			for _, t := range b.Cfg.Backups.SFTP {
				if _, err := t.Mirror(ctx, b.Cfg.Backups.Tag, keep); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", t.Name(), err))
				}
			}
			for _, t := range b.Cfg.Backups.S3 {
				if _, err := t.Mirror(ctx, b.Cfg.Backups.Tag, keep); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", t.Name(), err))
				}
			}
		}
	}
	return errors.Join(errs...)
}

// Game adapts the REST client for the routine.
type Game struct{ C *palworld.Client }

func (g Game) Announce(ctx context.Context, m string) error { return g.C.Announce(ctx, m) }
func (g Game) Save(ctx context.Context) error               { return g.C.Save(ctx) }
func (g Game) Shutdown(ctx context.Context, s int, m string) error {
	return g.C.Shutdown(ctx, s, m)
}
func (g Game) Alive(ctx context.Context) bool {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := g.C.Info(c)
	return err == nil
}

// Runner runs the routine on schedule or on request, one at a time.
type Runner struct {
	Cfg     config.Warden
	Game    routine.Game
	Host    *host.Host
	Backups *Backups
	Store   *store.Store
	Notify  *notify.Notifier
	Mods    *mods.Manager
	mu      sync.Mutex
	busy    bool
	stepNow string
}

// Begin claims the server for one operation (the routine, a restore, a mod change, a restart, a
// backup). Only one runs at a time; the second gets a clear "busy" error.
func (r *Runner) Begin(what string) (func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busy {
		return nil, fmt.Errorf("busy: %s is in progress; try again when it's done", r.stepNow)
	}
	r.busy, r.stepNow = true, what
	return func() { r.mu.Lock(); r.busy, r.stepNow = false, ""; r.mu.Unlock() }, nil
}

// Busy reports whether a run is in progress and which step it's on.
func (r *Runner) Busy() (bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.busy, r.stepNow
}

// RunNow runs the routine once (kind "nightly" or "manual"). warn overrides the warnings.
func (r *Runner) RunNow(ctx context.Context, kind string, warn []int, update bool) (*routine.Report, error) {
	end, err := r.Begin("the " + kind + " routine")
	if err != nil {
		return nil, err
	}
	defer end()
	p := routine.Plan{WarnMinutes: warn, Update: update}
	p.BeforeStart = r.WhileStopped
	rep := routine.Run(ctx, p, r.Game, r.Host, r.Backups)
	r.Store.AddRun(kind, rep.Started, rep.OK, rep)
	log.Printf("%s routine finished: ok=%v", kind, rep.OK)
	r.tell(kind, rep)
	return rep, nil
}

// WhileStopped makes the changes that can only happen with the server stopped: waiting mod
// changes, and waiting game-settings changes (Palworld rewrites its settings file on shutdown).
func (r *Runner) WhileStopped(context.Context) (string, error) {
	var parts []string
	var errs []error
	if r.Mods != nil {
		done, err := r.Mods.ApplyPending()
		if len(done) > 0 {
			parts = append(parts, "mods: "+strings.Join(done, ", "))
		}
		errs = append(errs, err)
	}
	sum, err := gamesettings.ApplyPending(config.SettingsPath(r.Cfg.ServerDir), r.Backups.DataDir)
	if sum != "" {
		parts = append(parts, sum)
	}
	errs = append(errs, err)
	sum, err = gamesettings.ApplyEngine(config.SettingsPath(r.Cfg.ServerDir), r.Backups.DataDir)
	if sum != "" {
		parts = append(parts, sum)
	}
	errs = append(errs, err)
	return strings.Join(parts, "; "), errors.Join(errs...)
}

// tell sends a run's result: problems always, a clean run only if the admin asked for that.
func (r *Runner) tell(kind string, rep *routine.Report) {
	if r.Notify == nil || (rep.OK && !r.Cfg.Notify.RoutineOK) {
		return
	}
	var bad []string
	for _, s := range rep.Steps {
		if !s.OK {
			bad = append(bad, s.Name+": "+s.Detail)
		}
	}
	if rep.OK {
		r.Notify.Send(context.Background(), notify.Info, "", "The "+kind+" routine went fine", strings.Join(rep.Backups, ", "))
		return
	}
	r.Notify.Send(context.Background(), notify.Warn, "", "The "+kind+" routine had problems", strings.Join(bad, "\n"))
}

// NextRun is when the stop happens; the warnings start before it.
func NextRun(timeOfDay string, now time.Time) time.Time {
	t, err := time.ParseInLocation("15:04", timeOfDay, now.Location())
	if err != nil {
		return time.Time{}
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// Schedule runs the nightly routine every day so that the stop lands at Nightly.Time.
func (r *Runner) Schedule(ctx context.Context) {
	if !r.Cfg.Nightly.Enabled {
		return
	}
	lead := 0 // the longest warning, whatever order they were typed in
	for _, m := range r.Cfg.Nightly.WarnMinutes {
		lead = max(lead, m)
	}
	for {
		stop := NextRun(r.Cfg.Nightly.Time, time.Now().Add(time.Duration(lead)*time.Minute))
		begin := stop.Add(-time.Duration(lead) * time.Minute)
		log.Printf("nightly routine: next at %s (warnings from %s)", stop.Format("Mon 15:04"), begin.Format("15:04"))
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(begin)):
		}
		ran := false
		for try := 0; try < 90 && !ran; try++ { // another operation may hold the server: wait for it
			if _, err := r.RunNow(ctx, "nightly", r.Cfg.Nightly.WarnMinutes, r.Cfg.Update.Enabled); err == nil {
				ran = true
				break
			}
			time.Sleep(time.Minute)
		}
		if !ran {
			r.Store.AddEvent(time.Now(), "nightly", "the nightly routine didn't run: the server was busy for 90 minutes")
			if r.Notify != nil {
				r.Notify.Send(ctx, notify.Urgent, "", "The nightly routine didn't run", "The server was busy with something else for 90 minutes, so there's no backup or update tonight.")
			}
		}
		time.Sleep(time.Minute) // never twice in the same minute
	}
}
