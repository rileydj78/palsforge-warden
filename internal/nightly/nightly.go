// Package nightly connects the routine engine to this server: what goes into backups, where the
// copies go, when the routine runs, and the record of every run.
package nightly

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/backup"
	"github.com/rileydj78/palsforge-warden/internal/config"
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
func (b *Backups) CopyAndVerify(ctx context.Context, _ []string) error {
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
	return errors.Join(errs...)
}

// Prune applies retention locally and in every second place.
func (b *Backups) Prune(ctx context.Context) error {
	p := backup.Policy{Daily: b.Cfg.Backups.Daily, Weekly: b.Cfg.Backups.Weekly}
	var errs []error
	for _, dir := range append([]string{b.LocalDir()}, b.Cfg.Backups.Offsite...) {
		if _, err := backup.Prune(dir, p); err != nil {
			errs = append(errs, err)
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

// Busy reports whether a run is in progress and which step it's on.
func (r *Runner) Busy() (bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.busy, r.stepNow
}

// RunNow runs the routine once (kind "nightly" or "manual"). warn overrides the warnings.
func (r *Runner) RunNow(ctx context.Context, kind string, warn []int, update bool) (*routine.Report, error) {
	r.mu.Lock()
	if r.busy {
		r.mu.Unlock()
		return nil, errors.New("a run is already in progress")
	}
	r.busy = true
	r.mu.Unlock()
	defer func() { r.mu.Lock(); r.busy = false; r.stepNow = ""; r.mu.Unlock() }()
	p := routine.Plan{WarnMinutes: warn, Update: update}
	if r.Mods != nil {
		p.BeforeStart = func(context.Context) (string, error) {
			done, err := r.Mods.ApplyPending()
			return strings.Join(done, ", "), err
		}
	}
	rep := routine.Run(ctx, p, r.Game, r.Host, r.Backups)
	r.Store.AddRun(kind, rep.Started, rep.OK, rep)
	log.Printf("%s routine finished: ok=%v", kind, rep.OK)
	r.tell(kind, rep)
	return rep, nil
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
	lead := 0
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
		r.RunNow(ctx, "nightly", r.Cfg.Nightly.WarnMinutes, r.Cfg.Update.Enabled)
		time.Sleep(time.Minute) // never twice in the same minute
	}
}
