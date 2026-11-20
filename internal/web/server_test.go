package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rileydj78/palsforge-warden/internal/auth"
	"github.com/rileydj78/palsforge-warden/internal/palworld"
	"github.com/rileydj78/palsforge-warden/internal/store"
)

type fakeGame struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeGame) start(t *testing.T) *palworld.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		switch r.URL.Path {
		case "/v1/api/info":
			fmt.Fprint(w, `{"version":"v1.0.5","servername":"Test"}`)
		case "/v1/api/metrics":
			fmt.Fprint(w, `{"serverfps":60,"currentplayernum":1,"maxplayernum":32}`)
		case "/v1/api/players":
			fmt.Fprint(w, `{"players":[{"name":"Moss","userId":"steam_1","level":5,"ip":"10.0.0.9"}]}`)
		default:
			w.WriteHeader(200)
		}
	}))
	t.Cleanup(srv.Close)
	var port int
	fmt.Sscanf(strings.TrimPrefix(srv.URL, "http://127.0.0.1:"), "%d", &port)
	return palworld.New("127.0.0.1", port, "pw")
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
	csrf string
}

func (c *client) do(method, path string, body any, csrf bool) (int, map[string]any) {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf {
		req.Header.Set("X-CSRF", c.csrf)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func setup(t *testing.T) (*client, *fakeGame, *auth.Auth) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	au, _ := auth.New(st.DB)
	fg := &fakeGame{}
	s := &Server{Game: fg.start(t), Store: st, Auth: au, Version: "test"}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: ts.URL, http: &http.Client{Jar: jar}}, fg, au
}

func TestAdminPortNeedsAccounts(t *testing.T) {
	c, fg, au := setup(t)
	code, me := c.do("GET", "/api/me", nil, false)
	if code != 200 || me["user"] != nil || me["accounts"] != float64(0) {
		t.Fatalf("the admin port starts with nobody signed in and no accounts: %v", me)
	}
	if code, _ := c.do("GET", "/api/status", nil, false); code != 401 {
		t.Fatalf("status without sign-in must be refused, got %d", code)
	}
	if code, _ := c.do("POST", "/api/setup", map[string]string{"name": "eve", "password": "correct horse battery"}, false); code == 200 {
		t.Fatal("there is no remote setup: nobody can claim Warden from the admin port")
	}
	au.AddUser("moss", "correct horse battery", auth.Owner)
	code, out := c.do("POST", "/api/login", map[string]string{"name": "moss", "password": "correct horse battery"}, false)
	if code != 200 || out["csrf"] == nil {
		t.Fatalf("login failed: %d %v", code, out)
	}
	c.csrf = out["csrf"].(string)
	code, st := c.do("GET", "/api/status", nil, false)
	if code != 200 || st["online"] != true {
		t.Fatalf("status: %d %v", code, st)
	}
	if strings.Contains(fmt.Sprint(st), "10.0.0.9") {
		t.Fatal("player IP addresses must never reach the browser")
	}
	if code, _ := c.do("POST", "/api/action/announce", map[string]string{"message": "hi"}, false); code != 403 {
		t.Fatalf("missing CSRF must be refused, got %d", code)
	}
	for _, call := range fg.calls {
		if strings.Contains(call, "announce") {
			t.Fatal("the game was called without CSRF")
		}
	}
	if code, _ := c.do("POST", "/api/action/announce", map[string]string{"message": "Boss raid at 8"}, true); code != 200 {
		t.Fatalf("announce with CSRF should work, got %d", code)
	}
}

func TestLocalPortIsTheOwnerWithoutSignIn(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	au, _ := auth.New(st.DB)
	fg := &fakeGame{}
	s := &Server{Game: fg.start(t), Store: st, Auth: au, Version: "test"}
	ts := httptest.NewServer(localOnly(s.Handler()))
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: ts.URL, http: &http.Client{Jar: jar}}
	code, me := c.do("GET", "/api/me", nil, false)
	user, _ := me["user"].(map[string]any)
	if code != 200 || user["role"] != "owner" || me["local"] != true {
		t.Fatalf("the local port is the owner: %v", me)
	}
	c.csrf = me["csrf"].(string)
	if code, _ := c.do("POST", "/api/users", map[string]string{"name": "friend", "password": "another long password", "role": "admin"}, true); code != 200 {
		t.Fatalf("the owner adds remote accounts from the local port, got %d", code)
	}
	if code, _ := c.do("POST", "/api/action/save", map[string]string{}, false); code != 403 {
		t.Fatal("the local port still needs the CSRF token (other web pages on this machine)")
	}
	req, _ := http.NewRequest("GET", ts.URL+"/api/me", nil)
	req.Header.Set("X-Forwarded-For", "100.64.0.9")
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 403 {
		t.Fatal("a request forwarded from elsewhere must never get the local port's owner rights")
	}
}

func TestModeratorCannotBan(t *testing.T) {
	c, _, au := setup(t)
	au.AddUser("owner", "correct horse battery", auth.Owner)
	if err := au.AddUser("mod", "another long password", auth.Moderator); err != nil {
		t.Fatal(err)
	}
	code, out := c.do("POST", "/api/login", map[string]string{"name": "mod", "password": "another long password"}, false)
	if code != 200 {
		t.Fatalf("login: %d %v", code, out)
	}
	c.csrf = out["csrf"].(string)
	if code, _ := c.do("POST", "/api/action/ban", map[string]string{"userId": "steam_1"}, true); code != 403 {
		t.Fatalf("a moderator must not ban, got %d", code)
	}
	if code, _ := c.do("POST", "/api/action/kick", map[string]string{"userId": "steam_1", "message": "griefing"}, true); code != 200 {
		t.Fatalf("a moderator may kick, got %d", code)
	}
	if code, _ := c.do("GET", "/api/users", nil, false); code != 403 {
		t.Fatal("only the owner manages admins")
	}
}

func TestMapCoords(t *testing.T) {
	// The community transform: world (0,0) is about map (-341, 267).
	x, y := mapCoords(0, 0)
	if x != -341 || y != 267 {
		t.Fatalf("got %d,%d", x, y)
	}
}

func TestAllowListNeedsAdmin(t *testing.T) {
	c, _, au := setup(t)
	au.AddUser("mod", "another long password", auth.Moderator)
	_, out := c.do("POST", "/api/login", map[string]string{"name": "mod", "password": "another long password"}, false)
	c.csrf = out["csrf"].(string)
	if code, _ := c.do("POST", "/api/allow", map[string]any{"userId": "steam_1", "on": true}, true); code != 403 {
		t.Fatalf("a moderator must not change the allow-list, got %d", code)
	}
	if code, _ := c.do("GET", "/api/allow", nil, false); code != 200 {
		t.Fatalf("a moderator may see it, got %d", code)
	}
}

func TestPublicPageShowsOnlyStatus(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	au, _ := auth.New(st.DB)
	fg := &fakeGame{}
	s := &Server{Game: fg.start(t), Store: st, Auth: au, Version: "test", Public: &Public{Invite: "https://discord.gg/x"}}
	ts := httptest.NewServer(s.PublicHandler())
	t.Cleanup(ts.Close)
	resp, _ := http.Get(ts.URL + "/")
	b := new(bytes.Buffer)
	b.ReadFrom(resp.Body)
	page := b.String()
	if !strings.Contains(page, "Test") || !strings.Contains(page, "Online") || strings.Contains(page, "Moss") || strings.Contains(page, "10.0.0.9") || strings.Contains(page, "steam_1") {
		t.Fatalf("name and state only; no names unless asked, never IPs or IDs: %s", page)
	}
	for _, p := range []string{"/api/me", "/api/status", "/api/users", "/index.html"} {
		if r, _ := http.Get(ts.URL + p); r.StatusCode != 404 {
			t.Fatalf("%s must not exist on the public port (%d)", p, r.StatusCode)
		}
	}
	if r, _ := http.Post(ts.URL+"/", "text/plain", nil); r.StatusCode == 200 {
		t.Fatal("the public port is read-only")
	}
}

func TestModeratorCannotRestart(t *testing.T) {
	c, _, au := setup(t)
	au.AddUser("mod", "another long password", auth.Moderator)
	_, out := c.do("POST", "/api/login", map[string]string{"name": "mod", "password": "another long password"}, false)
	c.csrf = out["csrf"].(string)
	if code, _ := c.do("POST", "/api/server/restart", map[string]int{"minutes": 0}, true); code < 400 { // 403 with server control; no route at all without it
		t.Fatalf("a moderator must not restart the server, got %d", code)
	}
}
