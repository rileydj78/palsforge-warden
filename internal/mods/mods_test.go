package mods

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mk(t *testing.T, root string, files ...string) {
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
}

func TestListProblemsAndToggle(t *testing.T) {
	srv, data := t.TempDir(), t.TempDir()
	mk(t, srv,
		"Pal/Content/Paks/~mods/BetterStacks_P.pak",
		"Pal/Binaries/Win64/ue4ss/UE4SS.dll",
		"Pal/Binaries/Win64/ue4ss/Mods/shared/types.lua",
		"Pal/Binaries/Win64/ue4ss/Mods/FastTravel/Scripts/main.lua",
		"Pal/Binaries/Win64/ue4ss/Mods/Nested/FastTravel/Scripts/main.lua",
		"Pal/Binaries/Win64/ue4ss/Mods/MisplacedSchema/mod.json",
		"Pal/Binaries/Win64/PalDefender/Config.json",
	)
	m := &Manager{ServerDir: srv, DataDir: data}
	list, probs, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, md := range list {
		names[md.ID] = md.Enabled
	}
	for _, want := range []string{"pak/BetterStacks_P.pak", "ue4ss/FastTravel", "ue4ss/Nested", "ue4ss/MisplacedSchema", "plugin/PalDefender"} {
		if !names[want] {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	if _, ok := names["ue4ss/shared"]; ok {
		t.Fatal("UE4SS's own folders aren't mods")
	}
	joined := ""
	for _, p := range probs {
		joined += p.Mod + " " + p.Text + "\n"
	}
	if !strings.Contains(joined, "MisplacedSchema looks like a PalSchema mod") || !strings.Contains(joined, "Nested has no Scripts/main.lua") {
		t.Fatalf("problems:\n%s", joined)
	}
	// Disable: queued, applied while stopped, shown as off; then enable again.
	if err := m.Queue("pak/BetterStacks_P.pak", "disable"); err != nil {
		t.Fatal(err)
	}
	list, _, _ = m.List()
	for _, md := range list {
		if md.ID == "pak/BetterStacks_P.pak" && md.Pending != "disable" {
			t.Fatal("should be waiting to be disabled")
		}
	}
	if done, err := m.ApplyPending(); err != nil || len(done) != 1 {
		t.Fatalf("%v %v", done, err)
	}
	if exists(filepath.Join(srv, "Pal/Content/Paks/~mods/BetterStacks_P.pak")) || !exists(filepath.Join(data, "mods-disabled/pak/BetterStacks_P.pak")) {
		t.Fatal("the mod should have moved to the disabled area")
	}
	m.Queue("pak/BetterStacks_P.pak", "enable")
	m.ApplyPending()
	if !exists(filepath.Join(srv, "Pal/Content/Paks/~mods/BetterStacks_P.pak")) {
		t.Fatal("enabling should move it back")
	}
}

func TestQueueRefusesUnknownAndPathTricks(t *testing.T) {
	m := &Manager{ServerDir: t.TempDir(), DataDir: t.TempDir()}
	if m.Queue("pak/../../Windows", "disable") == nil {
		t.Fatal("unknown mods must be refused")
	}
}

func zipOf(t *testing.T, files map[string]string) string {
	p := filepath.Join(t.TempDir(), "mod.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	f.Close()
	return p
}

func TestInstallKinds(t *testing.T) {
	srv, data := t.TempDir(), t.TempDir()
	m := &Manager{ServerDir: srv, DataDir: data}
	// A UE4SS mod nested one level too deep is found by its Scripts/main.lua.
	id, err := m.Install(zipOf(t, map[string]string{"FastTravel-1.2/FastTravel/Scripts/main.lua": "print(1)", "FastTravel-1.2/README.txt": "x"}), "FastTravel-1.2.zip")
	if err != nil || id != "ue4ss/FastTravel" {
		t.Fatalf("%s %v", id, err)
	}
	id, err = m.Install(zipOf(t, map[string]string{"BetterStacks_P.pak": "pak", "readme.txt": "x"}), "BetterStacks.zip")
	if err != nil || id != "pak/BetterStacks_P.pak" {
		t.Fatalf("%s %v", id, err)
	}
	if _, err := m.ApplyPending(); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(srv, "Pal/Binaries/Win64/ue4ss/Mods/FastTravel/Scripts/main.lua")) || !exists(filepath.Join(srv, "Pal/Content/Paks/~mods/BetterStacks_P.pak")) {
		t.Fatal("installed mods should be in place after applying")
	}
	if _, err := m.Install(zipOf(t, map[string]string{"notes.txt": "x"}), "x.zip"); err == nil || !strings.Contains(err.Error(), "can't tell") {
		t.Fatalf("unknown archives must be explained: %v", err)
	}
	if _, err := m.Install(zipOf(t, map[string]string{"../../evil/Scripts/main.lua": "x"}), "evil.zip"); err == nil {
		t.Fatal("paths that climb out must be refused")
	}
	if _, err := m.Install(zipOf(t, map[string]string{"BetterStacks_P.pak": "pak"}), "again.zip"); err == nil {
		t.Fatal("a second copy of an installed mod must be refused")
	}
}
