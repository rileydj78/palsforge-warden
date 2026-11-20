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
