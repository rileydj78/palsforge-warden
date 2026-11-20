package gamesettings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFile(t *testing.T) {
	dir := t.TempDir()
	try := func(text string) []string {
		p := filepath.Join(dir, "PalWorldSettings.ini")
		os.WriteFile(p, []byte(text), 0o644)
		return CheckFile(p)
	}
	good := "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(ExpRate=2.000000,ServerName=\"A, B\",bIsPvP=True)\n"
	if p := try(good); len(p) != 0 {
		t.Fatalf("a good file: %v", p)
	}
	for name, text := range map[string]string{
		"unclosed":     "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(ExpRate=2.000000,ServerName=\"A\"\n",
		"no header":    "OptionSettings=(ExpRate=2.000000)\n",
		"bad value":    "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(ExpRate=lots)\n",
		"twice":        "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(ExpRate=2.000000,ExpRate=3.000000)\n",
		"stray quote":  "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(ServerName=\"A,bIsPvP=True)\n",
		"two sections": "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(ExpRate=2.000000)\nOptionSettings=(ExpRate=3.000000)\n",
	} {
		if p := try(text); len(p) == 0 {
			t.Errorf("%s: should be reported", name)
		} else if strings.Contains(strings.Join(p, " "), "%!") {
			t.Errorf("%s: garbled message %v", name, p)
		}
	}
}
