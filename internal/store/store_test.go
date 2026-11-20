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
