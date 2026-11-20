package discordbot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const (
	guild    = "111111111111111111"
	modRole  = "222222222222222222"
	admRole  = "333333333333333333"
	someUser = "444444444444444444"
)

func TestCheckAndRoles(t *testing.T) {
	c := Config{Token: "t", Guild: guild, ModRoles: []string{modRole}, AdminRoles: []string{admRole}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if (Config{}).Check() != nil || (Config{Token: "t"}).Check() == nil || (Config{Token: "t", Guild: "abc", ModRoles: []string{modRole}}).Check() == nil {
		t.Fatal("bad configs should fail, empty should pass")
	}
	if (Config{Token: "t", Guild: guild}).Check() == nil {
		t.Fatal("no roles at all should be refused")
	}
	if c.RoleFor([]string{"9", modRole}) != RoleModerator || c.RoleFor([]string{modRole, admRole}) != RoleAdmin || c.RoleFor(nil) != RoleNone {
		t.Fatal("role mapping")
	}
}

// fakeDiscord is a gateway and REST API in one test server.
type fakeDiscord struct {
	t         *testing.T
	srv       *httptest.Server
	mu        sync.Mutex
	commands  []map[string]any
	callbacks []map[string]any
	edits     []map[string]any
	toBot     chan map[string]any
	identify  chan map[string]any
}

func newFake(t *testing.T) *fakeDiscord {
	f := &fakeDiscord{t: t, toBot: make(chan map[string]any, 10), identify: make(chan map[string]any, 1)}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeDiscord) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ws/" {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ctx := r.Context()
		send := func(v any) { b, _ := json.Marshal(v); _ = c.Write(ctx, websocket.MessageText, b) }
		send(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 30000}})
		_, b, err := c.Read(ctx)
		if err != nil {
			return
		}
		var id map[string]any
		_ = json.Unmarshal(b, &id)
		f.identify <- id
		send(map[string]any{"op": 0, "s": 1, "t": "READY", "d": map[string]any{"session_id": "s1", "resume_gateway_url": "ws://unused", "application": map[string]any{"id": "app1"}, "user": map[string]any{"username": "Warden"}}})
		go func() {
			for {
				if _, _, err := c.Read(ctx); err != nil {
					return
				}
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-f.toBot:
				send(m)
			}
		}
	}
	if r.Header.Get("Authorization") != "Bot good-token" {
		w.WriteHeader(401)
		io.WriteString(w, `{"message":"401: Unauthorized"}`)
		return
	}
	var body map[string]any
	data, _ := io.ReadAll(r.Body)
	switch {
	case r.URL.Path == "/gateway/bot":
		io.WriteString(w, `{"url":"ws://`+r.Host+`/ws"}`)
	case r.Method == "PUT" && r.URL.Path == "/applications/app1/guilds/"+guild+"/commands":
		var cmds []map[string]any
		_ = json.Unmarshal(data, &cmds)
		f.mu.Lock()
		f.commands = cmds
		f.mu.Unlock()
		io.WriteString(w, `[]`)
	case strings.HasPrefix(r.URL.Path, "/interactions/"):
		_ = json.Unmarshal(data, &body)
		f.mu.Lock()
		f.callbacks = append(f.callbacks, body)
		f.mu.Unlock()
		w.WriteHeader(204)
	case strings.HasPrefix(r.URL.Path, "/webhooks/app1/"):
		_ = json.Unmarshal(data, &body)
		f.mu.Lock()
		f.edits = append(f.edits, body)
		f.mu.Unlock()
		io.WriteString(w, `{}`)
	default:
		f.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		w.WriteHeader(404)
	}
}

func (f *fakeDiscord) waitCallbacks(n int) []map[string]any {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		if len(f.callbacks) >= n {
			out := append([]map[string]any(nil), f.callbacks...)
			f.mu.Unlock()
			return out
		}
		f.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	f.t.Fatalf("waited for %d answers", n)
	return nil
}

type call struct {
	role, who, method, path string
	body                    any
}

func interact(kind int, id, user string, roles []string, data map[string]any) map[string]any {
	return map[string]any{"op": 0, "s": 2, "t": "INTERACTION_CREATE", "d": map[string]any{
		"id": id, "type": kind, "token": "tok-" + id, "guild_id": guild, "channel_id": "555555555555555555",
		"member": map[string]any{"roles": roles, "user": map[string]any{"id": user, "username": "pat"}}, "data": data}}
}

func content(m map[string]any) string {
	d, _ := m["data"].(map[string]any)
	s, _ := d["content"].(string)
	return s
}

func TestCommandsRunThroughWardenWithRoles(t *testing.T) {
	f := newFake(t)
	var mu sync.Mutex
	var calls []call
	b := &Bot{Cfg: Config{Token: "good-token", Guild: guild, ModRoles: []string{modRole}, AdminRoles: []string{admRole}}, API: f.srv.URL,
		Call: func(_ context.Context, role, who, method, path string, body any) (int, []byte) {
			mu.Lock()
			calls = append(calls, call{role, who, method, path, body})
			mu.Unlock()
			if path == "/api/status" {
				return 200, []byte(`{"online":true,"server":{"servername":"Palsforge","version":"v0.7"},"metrics":{"serverfps":60,"maxplayernum":32,"days":12},"players":[{"name":"Rex","level":30,"ping":40,"userId":"steam_1"}]}`)
			}
			return 200, []byte(`{"ok":true}`)
		}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.Run(ctx)
	id := <-f.identify
	if id["op"].(float64) != 2 {
		t.Fatal("expected identify")
	}
	for i := 0; i < 200 && !strings.HasPrefix(b.State(), "connected"); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.HasPrefix(b.State(), "connected") {
		t.Fatalf("state %q", b.State())
	}
	f.mu.Lock()
	if len(f.commands) != len(commands) {
		t.Fatalf("registered %d commands", len(f.commands))
	}
	f.mu.Unlock()

	// Anyone: /status, public.
	f.toBot <- interact(2, "1", someUser, nil, map[string]any{"name": "status"})
	cb := f.waitCallbacks(1)
	if !strings.Contains(content(cb[0]), "Palsforge") || !strings.Contains(content(cb[0]), "1/32") {
		t.Fatalf("status: %v", cb[0])
	}
	// No role: /kick refused, nothing called but autocomplete stays empty too.
	f.toBot <- interact(2, "2", someUser, nil, map[string]any{"name": "kick", "options": []any{map[string]any{"name": "player", "value": "steam_1"}}})
	cb = f.waitCallbacks(2)
	if !strings.Contains(content(cb[1]), "roles don't allow") {
		t.Fatalf("kick without role: %v", cb[1])
	}
	// Moderator: /kick runs as moderator with the player's ID and name.
	f.toBot <- interact(2, "3", someUser, []string{modRole}, map[string]any{"name": "kick", "options": []any{map[string]any{"name": "player", "value": "steam_1"}, map[string]any{"name": "reason", "value": "afk"}}})
	cb = f.waitCallbacks(3)
	if content(cb[2]) != "Kicked Rex." {
		t.Fatalf("kick: %v", cb[2])
	}
	mu.Lock()
	last := calls[len(calls)-1]
	mu.Unlock()
	if last.path != "/api/action/kick" || last.role != RoleModerator || last.who != "discord:pat" || last.body.(map[string]any)["userId"] != "steam_1" {
		t.Fatalf("kick call %+v", last)
	}
	// Moderator can't ban.
	f.toBot <- interact(2, "4", someUser, []string{modRole}, map[string]any{"name": "ban", "options": []any{map[string]any{"name": "player", "value": "Rex"}}})
	cb = f.waitCallbacks(4)
	if !strings.Contains(content(cb[3]), "roles don't allow") {
		t.Fatalf("mod ban: %v", cb[3])
	}
	// Admin: /ban asks first; someone else's button is refused; the asker's runs it.
	f.toBot <- interact(2, "5", someUser, []string{admRole}, map[string]any{"name": "ban", "options": []any{map[string]any{"name": "player", "value": "Rex"}}})
	cb = f.waitCallbacks(5)
	if !strings.HasPrefix(content(cb[4]), "Are you sure?") {
		t.Fatalf("ban confirm: %v", cb[4])
	}
	comps := cb[4]["data"].(map[string]any)["components"].([]any)[0].(map[string]any)["components"].([]any)
	yes := comps[0].(map[string]any)["custom_id"].(string)
	mu.Lock()
	before := len(calls)
	mu.Unlock()
	f.toBot <- interact(3, "6", "999999999999999999", []string{admRole}, map[string]any{"custom_id": yes})
	cb = f.waitCallbacks(6)
	if !strings.Contains(content(cb[5]), "Only the person who asked") {
		t.Fatalf("other user's button: %v", cb[5])
	}
	f.toBot <- interact(3, "7", someUser, []string{admRole}, map[string]any{"custom_id": yes})
	cb = f.waitCallbacks(7)
	if content(cb[6]) != "Banned Rex." || cb[6]["type"].(float64) != 7 {
		t.Fatalf("ban: %v", cb[6])
	}
	mu.Lock()
	var bans int
	for _, c := range calls[before:] {
		if c.path == "/api/action/ban" {
			bans++
			if c.role != RoleAdmin {
				t.Errorf("ban ran as %s", c.role)
			}
		}
	}
	mu.Unlock()
	if bans != 1 {
		t.Fatalf("%d bans", bans)
	}
	// The same button can't be used twice.
	f.toBot <- interact(3, "8", someUser, []string{admRole}, map[string]any{"custom_id": yes})
	cb = f.waitCallbacks(8)
	if !strings.Contains(content(cb[7]), "expired") {
		t.Fatalf("reused button: %v", cb[7])
	}
	// Slow command: deferred, then the original message is edited.
	f.toBot <- interact(2, "9", someUser, []string{admRole}, map[string]any{"name": "backup"})
	cb = f.waitCallbacks(9)
	if cb[8]["type"].(float64) != 5 {
		t.Fatalf("backup should defer: %v", cb[8])
	}
	for i := 0; i < 200; i++ {
		f.mu.Lock()
		n := len(f.edits)
		f.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.edits) != 1 || f.edits[0]["content"] != "Backup made." {
		t.Fatalf("backup edit: %v", f.edits)
	}
}

func TestWrongTokenStopsForGood(t *testing.T) {
	f := newFake(t)
	b := &Bot{Cfg: Config{Token: "bad", Guild: guild}, API: f.srv.URL, Call: func(context.Context, string, string, string, string, any) (int, []byte) { return 500, nil }}
	done := make(chan struct{})
	go func() { b.Run(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a refused token should stop the bot, not retry forever")
	}
	if !strings.Contains(b.State(), "refused the bot token") {
		t.Fatalf("state %q", b.State())
	}
}
