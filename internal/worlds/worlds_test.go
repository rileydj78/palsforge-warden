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

func TestWorldOptionSetAside(t *testing.T) {
	srv, data := t.TempDir(), t.TempDir()
	cfg := filepath.Join(srv, "Pal", "Saved", "Config", "WindowsServer")
	os.MkdirAll(cfg, 0o755)
	settings := filepath.Join(cfg, "PalWorldSettings.ini")
	id := "D701B76E4A3241AD755C9D87150366ED"
	w := filepath.Join(srv, "Pal", "Saved", "SaveGames", "0", id)
	os.MkdirAll(w, 0o755)
	os.WriteFile(filepath.Join(w, "Level.sav"), []byte("l"), 0o644)
	os.WriteFile(filepath.Join(w, "WorldOption.sav"), []byte("o"), 0o644)
	os.WriteFile(filepath.Join(cfg, "GameUserSettings.ini"), []byte("[/Script/Pal.PalGameLocalSettings]\r\nDedicatedServerName="+id+"\r\n"), 0o644)
	if OptionFile(srv, settings) == "" {
		t.Fatal("found")
	}
	if s, _ := ApplySetAsideOption(srv, settings, data); s != "" {
		t.Fatal("nothing happens unless asked")
	}
	QueueSetAsideOption(data)
	if s, err := ApplySetAsideOption(srv, settings, data); err != nil || s == "" || OptionFile(srv, settings) != "" {
		t.Fatal(s, err)
	}
	if m, _ := filepath.Glob(filepath.Join(w, "WorldOption.sav.warden-*.bak")); len(m) != 1 {
		t.Fatal("kept beside the world")
	}
}
