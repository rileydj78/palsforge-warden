package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSettings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "PalWorldSettings.ini")
	ini := "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(Difficulty=None,ServerName=\"My Server, Friends\",AdminPassword=\"s3cret\",PublicPort=8211,RESTAPIEnabled=True,RESTAPIPort=8212,CrossplayPlatforms=(Steam,Xbox,PS5,Mac),ExpRate=2.000000)\n"
	if err := os.WriteFile(p, []byte(ini), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := ReadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if !s.RESTEnabled || s.RESTPort != 8212 || s.AdminPassword != "s3cret" || s.PublicPort != 8211 {
		t.Fatalf("unexpected: %+v", s)
	}
	if s.ServerName != "My Server, Friends" {
		t.Fatalf("quoted comma broke parsing: %q", s.ServerName)
	}
	if s.All["CrossplayPlatforms"] != "(Steam,Xbox,PS5,Mac)" || s.All["ExpRate"] != "2.000000" {
		t.Fatalf("bracketed list broke parsing: %v", s.All)
	}
}

func TestMissingLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.ini")
	os.WriteFile(p, []byte("[nothing]\n"), 0o600)
	if _, err := ReadSettings(p); err == nil {
		t.Fatal("expected an error")
	}
}
