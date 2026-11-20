package store

import "testing"

func TestPlaytimeCountsOnlyNearbySightings(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.SawPlayer("steam_1", "B14D255A000000000000000000000000", "Moss", "Steam", 5, 1000, 180)
	s.SawPlayer("steam_1", "B14D255A000000000000000000000000", "Moss", "Steam", 6, 1060, 180) // +60 s
	s.SawPlayer("steam_1", "B14D255A000000000000000000000000", "Moss", "Steam", 6, 9000, 180) // a new session: no time added
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

func TestGamePlayersComeFromTheGame(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if NormUID("b14d255a-00000000-00000000-00000000") != "B14D255A000000000000000000000000" || NormUID("x") != "" {
		t.Fatal("NormUID")
	}
	s.SyncGamePlayers("W1", []GamePlayer{{UID: "AAAAAAAA000000000000000000000000", Name: "Apo", FirstJoined: 500}, {UID: "BBBBBBBB000000000000000000000000", FirstJoined: 900}})
	// A restore re-creates files later: the first-join time must not move later; a name stays once known.
	s.SyncGamePlayers("W1", []GamePlayer{{UID: "AAAAAAAA000000000000000000000000", FirstJoined: 800}})
	gs, _ := s.GamePlayers("W1")
	if len(gs) != 1 || gs[0].FirstJoined != 500 || gs[0].Name != "Apo" {
		t.Fatalf("after resync: %+v (a player whose save is gone drops out; first join and name kept)", gs)
	}
	if other, _ := s.GamePlayers("W2"); len(other) != 0 {
		t.Fatal("worlds are separate")
	}
}
