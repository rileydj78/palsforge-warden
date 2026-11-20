package events

import (
	"testing"
	"time"
)

func TestParseAndWindows(t *testing.T) {
	evs, err := Parse("Double XP weekend | Fri 18:00 | Sun 22:00 | ExpRate=2, PalCaptureRate=1.5\n# comment\nLate night loot | Sat 23:00 | Sun 01:00 | EnemyDropItemRate=5")
	if err != nil || len(evs) != 2 || evs[0].Settings["PalCaptureRate"] != "1.5" {
		t.Fatalf("%+v %v", evs, err)
	}
	fri := time.Date(2026, 11, 20, 19, 0, 0, 0, time.Local) // a Friday
	if !evs[0].Active(fri) || evs[0].Active(fri.Add(-2*time.Hour)) || evs[0].Active(fri.Add(52*time.Hour)) {
		t.Fatal("Friday 18:00 to Sunday 22:00")
	}
	sat2330 := time.Date(2026, 11, 21, 23, 30, 0, 0, time.Local)
	sun0030 := sat2330.Add(time.Hour)
	if !evs[1].Active(sat2330) || !evs[1].Active(sun0030) || evs[1].Active(sun0030.Add(time.Hour)) {
		t.Fatal("a window across midnight")
	}
	for _, bad := range []string{"x | Funday 18:00 | Sun 22:00 | ExpRate=2", "x | Fri 18:00 | Sun 22:00 |", "no bars here"} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("%q should be refused", bad)
		}
	}
}
