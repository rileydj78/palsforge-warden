package worlds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListAndSwitch(t *testing.T) {
	srv, data := t.TempDir(), t.TempDir()
	cfg := filepath.Join(srv, "Pal", "Saved", "Config", "WindowsServer")
	os.MkdirAll(cfg, 0o755)
	settings := filepath.Join(cfg, "PalWorldSettings.ini")
	a, b := "D701B76E4A3241AD755C9D87150366ED", "11111111222233334444555566667777"
	for _, id := range []string{a, b} {
		d := filepath.Join(srv, "Pal", "Saved", "SaveGames", "0", id, "Players")
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(filepath.Dir(d), "Level.sav"), []byte("level"), 0o644)
	}
	os.WriteFile(filepath.Join(srv, "Pal", "Saved", "SaveGames", "0", a, "Players", "x.sav"), []byte("p"), 0o644)
	os.WriteFile(filepath.Join(cfg, "GameUserSettings.ini"), []byte("[/Script/Pal.PalGameLocalSettings]\r\nDedicatedServerName="+a+"\r\n[ScalabilityGroups]\r\n"), 0o644)
	ws, _ := List(srv, settings, data)
	if len(ws) != 2 || ws[0].ID != a || !ws[0].Active || ws[0].Players != 1 {
		t.Fatalf("%+v", ws)
	}
	Queue(data, b)
	if Active(settings) != a {
		t.Fatal("nothing changes until the server is stopped")
	}
	if _, err := ApplyPending(settings, data); err != nil || Active(settings) != b || Pending(data) != "" {
		t.Fatal("switched while stopped")
	}
	got, _ := os.ReadFile(filepath.Join(cfg, "GameUserSettings.ini"))
	if !strings.Contains(string(got), "[ScalabilityGroups]") {
		t.Fatal("the rest of the file is kept")
	}
	fresh := NewID()
	Queue(data, fresh)
	ApplyPending(settings, data)
	ws, _ = List(srv, settings, data)
	if !ws[0].Active || ws[0].ID != fresh || len(ws) != 3 {
		t.Fatalf("a fresh world shows as the active one to come, and both old worlds are kept: %+v", ws)
	}
	if Queue(data, "../../evil") == nil || SetLabel(data, "nope", "x") == nil {
		t.Fatal("only real world IDs")
	}
}
