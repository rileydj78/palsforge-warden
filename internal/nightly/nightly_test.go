package nightly

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/backup"
	"github.com/rileydj78/palsforge-warden/internal/config"
	"github.com/rileydj78/palsforge-warden/internal/host"
	"github.com/rileydj78/palsforge-warden/internal/store"
)

func TestNextRun(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 11, 19, 23, 10, 0, 0, loc)
	if got := NextRun("04:00", now); got != time.Date(2026, 11, 20, 4, 0, 0, 0, loc) {
		t.Fatal(got)
	}
	if got := NextRun("23:30", now); got != time.Date(2026, 11, 19, 23, 30, 0, 0, loc) {
		t.Fatal(got)
	}
}

func TestBackupsWithOffsiteAndImageOnSunday(t *testing.T) {
	srv, data, off := t.TempDir(), t.TempDir(), t.TempDir()
	for p, c := range map[string]string{
		"Pal/Saved/SaveGames/0/W1/Level.sav":                  "world",
		"Pal/Saved/SaveGames/0/W1/backup/world/x/Level.sav":   "rolling",
		"Pal/Saved/Config/WindowsServer/PalWorldSettings.ini": "ini",
		"Pal/Content/Paks/~mods/cool.pak":                     "mod",
		"Pal/Binaries/Win64/PalServer-Win64-Shipping-Cmd.exe": "exe",
	} {
		f := filepath.Join(srv, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(f), 0o755)
		os.WriteFile(f, []byte(c), 0o644)
	}
	cfg := config.Defaults(srv)
	cfg.Backups.Offsite = []string{off}
	sunday := time.Date(2026, 11, 22, 4, 0, 0, 0, time.Local)
	b := &Backups{Cfg: cfg, DataDir: data, Now: func() time.Time { return sunday }}
	names, err := b.MakeAll(context.Background())
	if err != nil || len(names) != 2 || !strings.Contains(names[1], "-server-") {
		t.Fatalf("expected data + image on a Sunday: %v %v", names, err)
	}
	if err := b.CopyAndVerify(context.Background(), names); err != nil {
		t.Fatal(err)
	}
	got, _ := backup.List(off)
	if len(got) != 2 {
		t.Fatalf("offsite should hold both: %v", got)
	}
	monday := sunday.AddDate(0, 0, 1)
	b.Now = func() time.Time { return monday }
	if b.WantImage() {
		t.Fatal("weekly images only on Sundays")
	}
	b.Cfg.Backups.Offsite = nil
	if err := b.CopyAndVerify(context.Background(), nil); err == nil {
		t.Fatal("no second place must be reported, not silently accepted")
	}
}

type fakeGame struct{}

func (fakeGame) Announce(context.Context, string) error      { return nil }
func (fakeGame) Save(context.Context) error                  { return nil }
func (fakeGame) Shutdown(context.Context, int, string) error { return nil }
func (fakeGame) Alive(context.Context) bool                  { return true }

func TestRestoreRoundTrip(t *testing.T) {
	srv, data := t.TempDir(), t.TempDir()
	world := filepath.Join(srv, "Pal/Saved/SaveGames/0/W1/Level.sav")
	os.MkdirAll(filepath.Dir(world), 0o755)
	os.WriteFile(world, []byte("monday"), 0o644)
	cfg := config.Defaults(srv)
	cfg.Control.Mode, cfg.Control.StartCommand, cfg.Control.Exe = "external", []string{"true"}, "no-such-server-program"
	st, _ := store.Open(t.TempDir())
	b := &Backups{Cfg: cfg, DataDir: data, Now: func() time.Time { return time.Date(2026, 11, 16, 4, 0, 0, 0, time.Local) }}
	names, err := b.MakeAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(world, []byte("tuesday, griefed"), 0o644)
	b.Now = nil
	r := &Runner{Cfg: cfg, Game: fakeGame{}, Host: host.New(cfg, data), Backups: b, Store: st}
	a, err := r.FindArchive(0, names[0])
	if err != nil {
		t.Fatal(err)
	}
	rep, err := r.Restore(context.Background(), a, "test")
	if err != nil || !rep.OK {
		t.Fatalf("restore: %v %+v", err, rep.Steps)
	}
	if got, _ := os.ReadFile(world); string(got) != "monday" {
		t.Fatalf("world should be Monday's again, got %q", got)
	}
	kept, _ := filepath.Glob(filepath.Join(srv, "Pal", "Saved.before-restore-*"))
	if len(kept) != 1 {
		t.Fatal("the replaced world must be kept aside")
	}
	// A damaged backup is refused before anything is touched.
	os.WriteFile(a.Path, []byte("damaged"), 0o644)
	rep, _ = r.Restore(context.Background(), a, "test")
	if rep.OK || rep.Steps[0].Name != "check" {
		t.Fatalf("a damaged backup must be refused at the check: %+v", rep.Steps)
	}
}
