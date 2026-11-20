package mods

import (
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
