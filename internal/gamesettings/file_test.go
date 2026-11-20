package gamesettings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ini = "[/Script/Pal.PalGameWorldSettings]\r\nOptionSettings=(Difficulty=None,DayTimeSpeedRate=1.000000,ExpRate=1.000000,ServerName=\"Palsforge, PvE\",AdminPassword=\"secret\",RESTAPIEnabled=True,DeathPenalty=All,CrossplayPlatforms=(Steam,Xbox,PS5,Mac),bIsPvP=False,FutureOption=7)\r\n"

func write(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "PalWorldSettings.ini")
	os.WriteFile(p, []byte(ini), 0o644)
	return p
}

func TestReadKeepsOrderAndUnknowns(t *testing.T) {
	f, err := Read(write(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Pairs) != 10 || f.Pairs[9].Key != "FutureOption" || f.Values["ServerName"] != "Palsforge, PvE" || f.Values["CrossplayPlatforms"] != "(Steam,Xbox,PS5,Mac)" {
		t.Fatalf("%+v", f.Pairs)
	}
}

func TestPlanApplyRoundTrip(t *testing.T) {
	p := write(t)
	f, _ := Read(p)
	ch, err := f.Plan(map[string]string{"ExpRate": "2", "DayTimeSpeedRate": "1", "bIsPvP": "false", "DeathPenalty": "item", "ServerName": "Palsforge PvE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 3 { // ExpRate, DeathPenalty, ServerName; the others are unchanged
		t.Fatalf("changes: %+v", ch)
	}
	bak, err := f.Apply(ch)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	want := strings.Replace(strings.Replace(strings.Replace(ini, "ExpRate=1.000000", "ExpRate=2.000000", 1), "DeathPenalty=All", "DeathPenalty=Item", 1), `"Palsforge, PvE"`, `"Palsforge PvE"`, 1)
	if string(got) != want {
		t.Fatalf("file:\n%q\nwant\n%q", got, want)
	}
	if old, _ := os.ReadFile(bak); string(old) != ini {
		t.Fatal("the backup must hold the old file exactly")
	}
}

func TestPlanRefusesBadValues(t *testing.T) {
	f, _ := Read(write(t))
	for k, v := range map[string]string{"ExpRate": "500", "bIsPvP": "maybe", "DeathPenalty": "Everything", "AdminPassword": "new", "ServerName": `evil",AdminPassword="x`, "FutureOption": "1,2"} {
		if _, err := f.Plan(map[string]string{k: v}); err == nil {
			t.Fatalf("%s=%s should be refused", k, v)
		}
	}
}
