package nightly

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/backup"
	"github.com/rileydj78/palsforge-warden/internal/routine"
)

// FindArchive looks a backup up by name in one place (0 = this machine, 1.. = second places).
func (r *Runner) FindArchive(place int, name string) (backup.Archive, error) {
	dirs := append([]string{r.Backups.LocalDir()}, r.Cfg.Backups.Offsite...)
	if place < 0 || place >= len(dirs) {
		return backup.Archive{}, errors.New("no such backup place")
	}
	list, err := backup.ListTag(dirs[place], r.Cfg.Backups.Tag)
	if err != nil {
		return backup.Archive{}, err
	}
	for _, a := range list {
		if a.Name == name {
			return a, nil
		}
	}
	return backup.Archive{}, fmt.Errorf("%s isn't in %s", name, dirs[place])
}

// Restore puts a data backup back: check it, warn and stop the server, make a safety backup of
// what's there now, move the current folders aside (kept, named *.before-restore-<time>), unpack,
// and start the server. If unpacking fails, the moved folders are put back. The server is always
// started again at the end.
func (r *Runner) Restore(ctx context.Context, a backup.Archive, by string) (*routine.Report, error) {
	end, err := r.Begin("a restore")
	if err != nil {
		return nil, err
	}
	defer end()

	rep := &routine.Report{Started: time.Now(), OK: true, Backups: []string{a.Name}}
	step := func(name string, err error, detail string) bool {
		s := routine.Step{At: time.Now(), Name: name, OK: err == nil, Detail: detail}
		if err != nil {
			s.Detail = strings.TrimSpace(detail + " " + err.Error())
			rep.OK = false
		}
		rep.Steps = append(rep.Steps, s)
		return err == nil
	}
	finish := func() (*routine.Report, error) {
		step("start", r.Host.Start(ctx), "")
		rep.Finished = time.Now()
		r.Store.AddRun("restore", rep.Started, rep.OK, rep)
		r.tell("restore", rep)
		return rep, nil
	}
	if a.Kind != backup.Data {
		step("check", errors.New("only data backups (world, settings, mods) are restored from the panel; restore a whole-server image by unzipping it while the server is stopped"), "")
		rep.Finished = time.Now()
		r.Store.AddRun("restore", rep.Started, false, rep)
		return rep, nil
	}
	if !step("check", backup.Verify(a), "checksum") {
		rep.Finished = time.Now()
		r.Store.AddRun("restore", rep.Started, false, rep)
		return rep, nil
	}
	// Stop: warn, save is pointless (we're replacing the world), plan the stop, shut down.
	r.Game.Announce(ctx, "The server is restoring a backup and will restart in 30 seconds.")
	r.Host.PlanStop(ctx)
	if err := r.Game.Shutdown(ctx, 30, "Restoring a backup. Back in a few minutes."); err != nil || !r.Host.WaitStopped(ctx, 3*time.Minute) {
		step("force stop", r.Host.ForceStop(ctx), "")
		if !r.Host.WaitStopped(ctx, time.Minute) {
			step("stop", errors.New("the server wouldn't stop; nothing was restored"), "")
			return finish()
		}
	}
	step("stop", nil, "stopped")
	// Safety backup of the current state.
	safe := *r.Backups
	safe.Cfg.Backups.Image = "off"
	names, err := safe.MakeAll(ctx)
	if !step("safety backup", err, strings.Join(names, ", ")) {
		return finish()
	}
	// Move the current folders aside.
	stamp := time.Now().Format("20060102-150405")
	var moved [][2]string
	for _, p := range DataPaths {
		src := filepath.Join(r.Cfg.ServerDir, filepath.FromSlash(p))
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := src + ".before-restore-" + stamp
		if err := os.Rename(src, dst); err != nil {
			step("move aside", err, p)
			for _, m := range moved {
				os.Rename(m[1], m[0])
			}
			return finish()
		}
		moved = append(moved, [2]string{src, dst})
	}
	step("move aside", nil, fmt.Sprintf("%d folder(s) kept as *.before-restore-%s", len(moved), stamp))
	n, err := backup.Extract(a.Path, r.Cfg.ServerDir, DataPaths)
	if !step("restore", err, fmt.Sprintf("%d files from %s", n, a.Name)) {
		for _, p := range DataPaths { // remove the half-restored folders, put the originals back
			os.RemoveAll(filepath.Join(r.Cfg.ServerDir, filepath.FromSlash(p)))
		}
		for _, m := range moved {
			os.Rename(m[1], m[0])
		}
		step("roll back", nil, "the folders as they were before are back")
	}
	return finish()
}
