package files

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func setup(t *testing.T) (*Manager, string) {
	srv := t.TempDir()
	plat := "WindowsServer"
	if runtime.GOOS != "windows" {
		plat = "LinuxServer"
	}
	cfg := filepath.Join(srv, "Pal", "Saved", "Config", plat)
	pd := filepath.Join(srv, "Pal", "Binaries", "Win64", "PalDefender", "RESTAPI", "Tokens")
	os.MkdirAll(cfg, 0o755)
	os.MkdirAll(pd, 0o755)
	os.WriteFile(filepath.Join(cfg, "Engine.ini"), []byte("[Core.System]\n"), 0o644)
	os.WriteFile(filepath.Join(pd, "Warden.json"), []byte(`{"Token":"secret"}`), 0o644)
	os.MkdirAll(filepath.Join(srv, "Pal", "Saved", "SaveGames"), 0o755)
	return &Manager{Roots: Roots(srv), DataDir: t.TempDir()}, srv
}

func TestOnlyInsideTheRoots(t *testing.T) {
	m, srv := setup(t)
	if len(m.Roots) != 2 {
		t.Fatalf("roots: %+v", m.Roots)
	}
	for _, bad := range []string{"../../SaveGames", "/etc/passwd", "..", "a/../../x.ini", `C:\Windows\win.ini`} {
		if _, err := m.List("config", bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
	if _, err := m.List("saves", ""); err == nil {
		t.Error("unknown roots must be refused")
	}
	if _, _, err := m.ReadText("paldefender", "RESTAPI/Tokens/Warden.json"); err == nil || !strings.Contains(err.Error(), "secrets") {
		t.Errorf("tokens must stay hidden: %v", err)
	}
	ls, _ := m.List("paldefender", "RESTAPI")
	if len(ls) != 0 {
		t.Errorf("the Tokens folder must not be listed: %+v", ls)
	}
	if runtime.GOOS != "windows" { // a link that points out of the root
		cfg := m.Roots[0].Dir
		os.Symlink(filepath.Join(srv, "Pal", "Saved", "SaveGames"), filepath.Join(cfg, "escape"))
		if _, err := m.List("config", "escape"); err == nil {
			t.Error("a link out of the folder must be refused")
		}
		if err := m.WriteText("config", "escape/x.ini", "x", 0); err == nil {
			t.Error("writing through a link out of the folder must be refused")
		}
	}
}

func TestEditKeepsVersionsAndRefusesStaleSaves(t *testing.T) {
	m, _ := setup(t)
	text, mod, err := m.ReadText("config", "Engine.ini")
	if err != nil || text != "[Core.System]\n" {
		t.Fatal(text, err)
	}
	if err := m.WriteText("config", "Engine.ini", "[Core.System]\nA=1\n", mod); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteText("config", "Engine.ini", "B=2\n", mod); err == nil {
		t.Fatal("a save based on an old read must be refused")
	}
	vs, _ := m.Versions("config", "Engine.ini")
	if len(vs) != 1 {
		t.Fatalf("versions: %v", vs)
	}
	old, err := m.ReadVersion("config", "Engine.ini", vs[0])
	if err != nil || old != "[Core.System]\n" {
		t.Fatal(old, err)
	}
	if _, err := m.ReadVersion("config", "Engine.ini", "../../x"); err == nil {
		t.Fatal("version names can't be paths")
	}
	if err := m.WriteText("config", "game.exe", "x", 0); err == nil {
		t.Fatal("only text files can be edited")
	}
}

func TestUploadAndTrash(t *testing.T) {
	m, _ := setup(t)
	if err := m.Upload("config", "new/Custom.ini", strings.NewReader("x=1")); err != nil {
		t.Fatal(err)
	}
	dst, err := m.Trash("config", "new/Custom.ini")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "x=1" {
		t.Fatal("the bin must keep the file")
	}
	if _, err := m.Trash("config", ""); err == nil {
		t.Fatal("the root itself can't be deleted")
	}
	if err := m.Upload("config", ".hidden", strings.NewReader("x")); err == nil {
		t.Fatal("dot files are refused")
	}
}
