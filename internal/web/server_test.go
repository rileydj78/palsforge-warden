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

func TestFirstRunSignInAndCSRF(t *testing.T) {
	c, fg, au := setup(t)
	code, me := c.do("GET", "/api/me", nil, false)
	if code != 200 || me["setupNeeded"] != true {
		t.Fatalf("a fresh install should ask for setup: %v", me)
	}
	if code, _ := c.do("GET", "/api/status", nil, false); code != 401 {
		t.Fatalf("status without sign-in should be refused, got %d", code)
	}
	if code, _ := c.do("POST", "/api/setup", map[string]string{"code": "nope", "name": "moss", "password": "correct horse battery"}, false); code != 400 {
		t.Fatal("a wrong setup code must be refused")
	}
	code, out := c.do("POST", "/api/setup", map[string]string{"code": au.SetupCode, "name": "moss", "password": "correct horse battery"}, false)
	if code != 200 || out["csrf"] == nil {
		t.Fatalf("setup failed: %d %v", code, out)
	}
	c.csrf = out["csrf"].(string)
	code, st := c.do("GET", "/api/status", nil, false)
	if code != 200 || st["online"] != true {
		t.Fatalf("status: %d %v", code, st)
	}
	if strings.Contains(fmt.Sprint(st), "10.0.0.9") {
		t.Fatal("player IP addresses must never reach the browser")
	}
	// A change without the CSRF token is refused and never reaches the game.
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
	code, _ = c.do("GET", "/api/audit", nil, false)
	if code != 200 {
		t.Fatal("audit log should be readable")
	}
}

func TestModeratorCannotBan(t *testing.T) {
	c, _, au := setup(t)
	au.CreateOwner(au.SetupCode, "owner", "correct horse battery")
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
