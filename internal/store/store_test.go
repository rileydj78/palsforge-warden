package store

import "testing"

func TestPlaytimeCountsOnlyNearbySightings(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.SawPlayer("steam_1", "Moss", "Steam", 5, 1000, 180)
	s.SawPlayer("steam_1", "Moss", "Steam", 6, 1060, 180) // +60 s
	s.SawPlayer("steam_1", "Moss", "Steam", 6, 9000, 180) // a new session: no time added
	r, _ := s.Roster()
	if len(r) != 1 || r[0].Seconds != 60 || r[0].Level != 6 || r[0].FirstSeen != 1000 || r[0].LastSeen != 9000 {
		t.Fatalf("roster: %+v", r)
	}
}

func TestAllowList(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.Allow("steam_1", "owner", true)
	if !s.Allowed("steam_1") || s.Allowed("steam_2") {
		t.Fatal("allowed")
	}
	s.Allow("steam_1", "owner", false)
	if s.Allowed("steam_1") {
		t.Fatal("removed")
	}
}
