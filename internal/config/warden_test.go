package config

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultsAreValidAndRoundTrip(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	w := Defaults(dir)
	if err := w.Check(); err != nil {
		t.Fatalf("defaults should be valid: %v", err)
	}
	if _, found, err := Load(data, dir); err != nil || found {
		t.Fatalf("a missing warden.json is not an error: found=%v err=%v", found, err)
	}
	w.Nightly.Time = "03:30"
	w.Backups.Offsite = []string{`\\nas\backups\pal`}
	if err := Save(data, w); err != nil {
		t.Fatal(err)
	}
	got, found, err := Load(data, "")
	if err != nil || !found || got.Nightly.Time != "03:30" || got.Backups.Offsite[0] != `\\nas\backups\pal` || got.ServerDir != dir {
		t.Fatalf("round trip: %+v %v %v", got, found, err)
	}
}

func TestPartialFileKeepsDefaults(t *testing.T) {
	data := t.TempDir()
	os.WriteFile(ConfigPath(data), []byte(`{"nightly": {"time": "05:15"}}`), 0o600)
	w, _, err := Load(data, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if w.Nightly.Time != "05:15" || w.Backups.Daily != 14 || w.Control.Mode != "supervise" {
		t.Fatalf("unset values should keep their defaults: %+v", w)
	}
}

func TestCheckExplainsMistakes(t *testing.T) {
	w := Defaults(t.TempDir())
	w.Nightly.Time = "4am"
	w.Control.Mode = "external"
	w.Backups.Tag = "My Server"
	err := w.Check()
	if err == nil {
		t.Fatal("expected problems")
	}
	for _, want := range []string{"04:00", "startCommand", "lowercase"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("message should mention %q: %v", want, err)
		}
	}
}

func TestByteOrderMarkIsFine(t *testing.T) {
	data := t.TempDir()
	os.WriteFile(ConfigPath(data), []byte("\xef\xbb\xbf{\"nightly\": {\"time\": \"05:15\"}}"), 0o600)
	w, _, err := Load(data, t.TempDir())
	if err != nil || w.Nightly.Time != "05:15" {
		t.Fatalf("a file saved by Notepad must load: %v", err)
	}
}

func TestEventsAreCheckedWhenSaved(t *testing.T) {
	w := Defaults(t.TempDir())
	w.Events = "Double XP weekend | Fri 18:00 | Sun 22:00 | ExpRate=2, PalCaptureRate=1.5"
	if err := w.Check(); err != nil {
		t.Fatalf("a good event: %v", err)
	}
	w.Events = "Broken | Fri 18:00 | Sun 22:00 | ExpRate=lots"
	if err := w.Check(); err == nil || !strings.Contains(err.Error(), "Broken") {
		t.Fatalf("a bad value must be refused when saved, naming the event: %v", err)
	}
	w.Events = "Sneaky | Fri 18:00 | Sun 22:00 | AdminPassword=x"
	if w.Check() == nil {
		t.Fatal("events can't change locked settings")
	}
}
