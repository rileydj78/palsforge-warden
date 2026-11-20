package instances

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAnnounceListAndPorts(t *testing.T) {
	d := t.TempDir()
	Announce(d, Entry{Name: "", ServerName: "Main", Local: "127.0.0.1:8282", Admin: "127.0.0.1:8283", Public: "127.0.0.1:8284"})
	Announce(d, Entry{Name: "pvp", ServerName: "PvP", Local: "127.0.0.1:8292", Admin: "127.0.0.1:8293", Public: "127.0.0.1:8294"})
	old := Entry{Name: "gone", Local: "127.0.0.1:8302"}
	Announce(d, old)
	past := time.Now().Add(-10 * time.Minute)
	os.Chtimes(filepath.Join(Dir(d), "gone.json"), past, past)
	b, _ := os.ReadFile(filepath.Join(Dir(d), "gone.json"))
	os.WriteFile(filepath.Join(Dir(d), "gone.json"), []byte(string(b[:len(b)-1])+"x"), 0o644) // corrupt: ignored
	l := List(d)
	if len(l) != 2 || l[0].ServerName != "Main" || l[1].Name != "pvp" {
		t.Fatalf("main first, live ones only: %+v", l)
	}
	loc, adm, pub := Ports(d)
	if loc == "127.0.0.1:8292" || loc == "" || adm == "" || pub == "" {
		t.Fatalf("a new instance must not take a known instance's ports: %s %s %s", loc, adm, pub)
	}
	if ValidName("PvP Server") == nil || ValidName("pvp-2") != nil {
		t.Fatal("names")
	}
}
