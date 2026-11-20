package paldefender

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectKeepsSettingsAndLocksToLoopback(t *testing.T) {
	srv := t.TempDir()
	dir := filepath.Join(Dir(srv), "RESTAPI")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "RESTConfig.json"), []byte(`{"Version":3,"Enabled":false,"LogConsole":false,"Address":"0.0.0.0","Port":17993,"DefaultPermissions":[],"Cors":{"Allowed-Origins":"*"}}`), 0o644)
	cfg, err := Connect(srv)
	if err != nil || cfg.Token == "" || cfg.URL != "http://127.0.0.1:17993" {
		t.Fatalf("%+v %v", cfg, err)
	}
	var rc map[string]any
	b, _ := os.ReadFile(filepath.Join(dir, "RESTConfig.json"))
	json.Unmarshal(b, &rc)
	if rc["Enabled"] != true || rc["Address"] != "127.0.0.1" || rc["Version"] != float64(3) || rc["Cors"] == nil {
		t.Fatalf("config: %v", rc)
	}
	var tok map[string]any
	b, _ = os.ReadFile(filepath.Join(dir, "Tokens", "Warden.json"))
	json.Unmarshal(b, &tok)
	if tok["Token"] != cfg.Token {
		t.Fatal("the token file must hold Warden's token")
	}
}

func TestClient(t *testing.T) {
	var auth, body string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/v1/pdapi/version":
			w.Write([]byte(`{"Version":{"Major":1,"Version":"1.7.2","VersionLong":"1.7.2 (build 77)","Beta":false}}`))
		case "/v1/pdapi/players":
			w.Write([]byte(`{"Meta":{"PlayerCount":1,"OnlineCount":1},"Players":[{"Name":"Moss","IP":"10.0.0.9","PlayerUID":"A","UserId":"steam_1","GuildName":"Mossy Hollow","Status":"Online","MapLocation":{"x":-120.5,"y":344.2,"z":0}}]}`))
		case "/v1/pdapi/guilds":
			w.Write([]byte(`{"Meta":{"GuildCount":1},"Guilds":{"G-1":{"name":"Mossy Hollow","Level":7,"admin":{"id":"A","name":"Moss"},"camp_count":1,"camps":[{"id":"C1","map_pos":{"x":-120,"y":344,"z":0}}],"member_count":2,"members":{}}}}`))
		case "/v1/pdapi/ban/steam_1":
			var m map[string]any
			json.NewDecoder(r.Body).Decode(&m)
			b, _ := json.Marshal(m)
			body = string(b)
		}
	}))
	defer s.Close()
	c := New(Config{URL: s.URL, Token: "t0k"})
	v, err := c.Version(context.Background())
	if err != nil || v.Version != "1.7.2" || auth != "Bearer t0k" {
		t.Fatalf("%+v %v %q", v, err, auth)
	}
	ps, err := c.Players(context.Background())
	if err != nil || len(ps) != 1 || ps[0].GuildName != "Mossy Hollow" || ps[0].MapLocation.Y != 344.2 {
		t.Fatalf("%+v %v", ps, err)
	}
	if err := c.Ban(context.Background(), "steam_1", "cheating", true); err != nil || body != `{"IP":true,"Reason":"cheating"}` {
		t.Fatalf("%v %s", err, body)
	}
	gs, err := c.Guilds(context.Background())
	if err != nil || len(gs) != 1 || gs[0].ID != "G-1" || gs[0].Camps[0].MapPos.Y != 344 || gs[0].Admin.Name != "Moss" {
		t.Fatalf("guilds: %+v %v", gs, err)
	}
	if New(Config{}) != nil {
		t.Fatal("no token means no client")
	}
}

func TestScrubRemovesAddresses(t *testing.T) {
	got := string(Scrub([]byte(`{"Name":"Moss","IP":"73.1.2.3","Equipment":[{"Note":"from 10.0.0.9"}],"PlayerIP":"x"}`)))
	if strings.Contains(got, "73.1.2.3") || strings.Contains(got, "10.0.0.9") || strings.Contains(got, "PlayerIP") || !strings.Contains(got, "Equipment") {
		t.Fatal(got)
	}
}
