package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rileydj78/palsforge-warden/internal/config"
)

func TestStarterSettings(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "DefaultPalWorldSettings.ini"), []byte("[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(Difficulty=None,ServerName=\"Default Palworld Server\",AdminPassword=\"\",RESTAPIEnabled=False,RESTAPIPort=8212)\n"), 0o644)
	p, err := StarterSettings(dir, `My "Server", 2x`)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	s := string(b)
	if !strings.Contains(s, "RESTAPIEnabled=True") || strings.Contains(s, `AdminPassword=""`) || !strings.Contains(s, `ServerName="My Server 2x"`) {
		t.Fatal(s)
	}
	ini, err := config.ReadSettings(p)
	if err != nil || !ini.RESTEnabled || len(ini.AdminPassword) < 16 {
		t.Fatalf("Warden must be able to use what it wrote: %+v %v", ini, err)
	}
	os.WriteFile(p, []byte("OptionSettings=(ServerName=\"Mine\")"), 0o600)
	StarterSettings(dir, "x")
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "Mine") {
		t.Fatal("an existing settings file must never be overwritten")
	}
}
