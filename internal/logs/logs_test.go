package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanMasksIPsAndSecrets(t *testing.T) {
	if c := Clean(`notify: Post "https://discord.com/api/webhooks/123/abcSECRET": timeout from 2601:44:8100:1a2::5`); strings.Contains(c, "abcSECRET") || strings.Contains(c, "8100") {
		t.Fatalf("leaked: %s", c)
	}
	got := Clean("[2026.11.19] Moss joined from 73.12.44.9:52133 AdminPassword=hunter2 local 127.0.0.1")
	if strings.Contains(got, "73.12.44.9") || strings.Contains(got, "hunter2") {
		t.Fatalf("leaked: %s", got)
	}
	if !strings.Contains(got, "73.x.x.x") || !strings.Contains(got, "127.0.0.1") {
		t.Fatalf("over-masked: %s", got)
	}
}

func TestTailAndSearch(t *testing.T) {
	srv, data := t.TempDir(), t.TempDir()
	p := filepath.Join(srv, "Pal", "Saved", "Logs", "Pal.log")
	os.MkdirAll(filepath.Dir(p), 0o755)
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("line ")
		if i%100 == 0 {
			b.WriteString("joined ")
		}
		b.WriteString("x\n")
	}
	os.WriteFile(p, []byte(b.String()), 0o644)
	srcs := Sources(srv, data)
	if len(srcs) != 1 || srcs[0].ID != "game" {
		t.Fatalf("sources: %+v", srcs)
	}
	lines, _ := Tail(srcs[0], 50, "")
	if len(lines) != 50 {
		t.Fatal(len(lines))
	}
	hits, _ := Tail(srcs[0], 50, "JOINED")
	if len(hits) != 5 {
		t.Fatalf("search: %d", len(hits))
	}
}

func TestCheatLogIsASource(t *testing.T) {
	srv, data := t.TempDir(), t.TempDir()
	d := filepath.Join(srv, "Pal", "Binaries", "Win64", "PalDefender", "Logs", "Cheats")
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "20.11 10.00.00.log"), []byte("[10:00:00][warning] something\n"), 0o644)
	found := false
	for _, s := range Sources(srv, data) {
		found = found || s.ID == "cheats"
	}
	if !found {
		t.Fatal("PalDefender's cheat detections should be on the Logs page")
	}
}
