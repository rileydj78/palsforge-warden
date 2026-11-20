package gamesettings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEngineWaitsForStop(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	settings := filepath.Join(dir, "PalWorldSettings.ini")
	os.WriteFile(EnginePath(settings), []byte("[Core.Log]\n"), 0o644)
	if QueueEngine(data, "[/Script/OnlineSubsystemUtils.IpNetDriver]\nrm -rf /\n") == nil {
		t.Fatal("non-ini text must be refused")
	}
	if err := QueueEngine(data, "; tuned\n[/Script/OnlineSubsystemUtils.IpNetDriver]\nNetServerMaxTickRate=30\n"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(EnginePath(settings)); string(b) != "[Core.Log]\n" {
		t.Fatal("nothing is written until the server stops")
	}
	if sum, err := ApplyEngine(settings, data); err != nil || sum == "" {
		t.Fatal(sum, err)
	}
	b, _ := os.ReadFile(EnginePath(settings))
	if string(b) == "[Core.Log]\n" {
		t.Fatal("not written")
	}
	if _, ok := PendingEngine(data); ok {
		t.Fatal("the queue empties once written")
	}
	m, _ := filepath.Glob(EnginePath(settings) + ".warden-*.bak")
	if len(m) != 1 {
		t.Fatal("the old file is kept")
	}
}
