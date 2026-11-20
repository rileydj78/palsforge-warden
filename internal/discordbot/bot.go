// Package discordbot runs Warden's Discord command bot: slash commands in one Discord server,
// answered through Warden's own panel API with the role the member's Discord roles map to, so
// every command is checked and audited exactly like the panel. It never gets the owner's
// powers (settings, restores, accounts, keys), the same rule as API keys.
package discordbot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Config is the "discordBot" part of warden.json.
type Config struct {
	Token      string   `json:"token"`      // the bot's token (Discord Developer Portal → Bot)
	Guild      string   `json:"guild"`      // the Discord server (guild) ID the commands live in
	ModRoles   []string `json:"modRoles"`   // role IDs that act as Moderator: players, announce, kick
	AdminRoles []string `json:"adminRoles"` // role IDs that act as Admin: also ban, save, backup, start/stop/restart
	Channel    string   `json:"channel"`    // optional: only answer in this channel ID
}

var snowflake = regexp.MustCompile(`^\d{17,20}$`)

// Enabled says whether the bot should run.
func (c Config) Enabled() bool { return c.Token != "" && c.Guild != "" }

// Check validates the IDs (the token is checked by Discord when the bot connects).
func (c Config) Check() error {
	if c.Token == "" && c.Guild == "" {
		return nil
	}
	if c.Token == "" || c.Guild == "" {
		return errors.New("discordBot: give both the bot token and the Discord server ID")
	}
	if !snowflake.MatchString(c.Guild) {
		return errors.New("discordBot.guild must be a Discord server ID (17–20 digits; turn on Developer Mode, right-click the server, Copy Server ID)")
	}
	for _, r := range append(slices.Clone(c.ModRoles), c.AdminRoles...) {
		if !snowflake.MatchString(r) {
			return fmt.Errorf("discordBot: %q isn't a Discord role ID (17–20 digits)", r)
		}
	}
	if c.Channel != "" && !snowflake.MatchString(c.Channel) {
		return errors.New("discordBot.channel must be a Discord channel ID (17–20 digits) or empty")
	}
	if len(c.ModRoles)+len(c.AdminRoles) == 0 {
		return errors.New("discordBot: give at least one Moderator or Admin role, or nobody can use the commands beyond /status")
	}
	return nil
}

// Roles Warden understands (the same names as internal/auth).
const (
	RoleNone      = ""
	RoleModerator = "moderator"
	RoleAdmin     = "admin"
)

// RoleFor maps a member's Discord roles to a Warden role (the highest that matches).
func (c Config) RoleFor(roles []string) string {
	for _, r := range roles {
		if slices.Contains(c.AdminRoles, r) {
			return RoleAdmin
		}
	}
	for _, r := range roles {
		if slices.Contains(c.ModRoles, r) {
			return RoleModerator
		}
	}
	return RoleNone
}

// Call runs one panel API request as role on behalf of who ("discord:<name>") and returns the
// HTTP status and body. Warden's web server provides it.
type Call func(ctx context.Context, role, who, method, path string, body any) (int, []byte)

// Bot is one connection to Discord.
type Bot struct {
	Cfg  Config
	Call Call
	Log  func(format string, a ...any)
	API  string // Discord's REST base; tests point it elsewhere
	HTTP *http.Client

	mu      sync.Mutex
	state   string
	appID   string
	pending map[string]pendingAction
}

type pendingAction struct {
	user    string
	command string
	args    map[string]any
	expires time.Time
}

// State is a short line for the settings page: connected, or what went wrong.
func (b *Bot) State() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == "" {
		return "starting"
	}
	return b.state
}

func (b *Bot) setState(s string) {
	b.mu.Lock()
	b.state = s
	b.mu.Unlock()
}

func (b *Bot) logf(format string, a ...any) {
	if b.Log != nil {
		b.Log("discord bot: "+format, a...)
	}
}

// errFatal stops the bot for good: retrying can't help (wrong token, missing intent).
type errFatal struct{ msg string }

func (e errFatal) Error() string { return e.msg }

// Run connects and answers commands until ctx ends, reconnecting with back-off.
func (b *Bot) Run(ctx context.Context) {
	if b.API == "" {
		b.API = "https://discord.com/api/v10"
	}
	if b.HTTP == nil {
		b.HTTP = &http.Client{Timeout: 20 * time.Second}
	}
	b.pending = map[string]pendingAction{}
	wait := 5 * time.Second
	var res resume
	for ctx.Err() == nil {
		start := time.Now()
		err := b.session(ctx, &res)
		var fatal errFatal
		if errors.As(err, &fatal) {
			b.setState("stopped: " + fatal.msg)
			b.logf("%s", fatal.msg)
			return
		}
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > 2*time.Minute {
			wait = 5 * time.Second
		}
		b.setState(fmt.Sprintf("reconnecting (%v)", err))
		b.logf("disconnected: %v; reconnecting in %s", err, wait)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, 5*time.Minute)
	}
}

type resume struct {
	url, session string
	seq          int64
}

type frame struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
	S  *int64          `json:"s"`
	T  string          `json:"t"`
}

func (b *Bot) session(ctx context.Context, res *resume) error {
	url := res.url
	if url == "" {
		var g struct {
			URL string `json:"url"`
		}
		if code, err := b.rest(ctx, "GET", "/gateway/bot", nil, &g); err != nil {
			if code == 401 {
				return errFatal{"Discord refused the bot token: copy it again from the Developer Portal (Bot → Reset Token)"}
			}
			return err
		}
		url = g.URL
	}
	c, _, err := websocket.Dial(ctx, url+"/?v=10&encoding=json", nil)
	if err != nil {
		return err
	}
	c.SetReadLimit(8 << 20)
	defer c.CloseNow()
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var writeMu sync.Mutex
	send := func(op int, d any) error {
		buf, _ := json.Marshal(map[string]any{"op": op, "d": d})
		writeMu.Lock()
		defer writeMu.Unlock()
		wctx, wcancel := context.WithTimeout(sctx, 10*time.Second)
		defer wcancel()
		return c.Write(wctx, websocket.MessageText, buf)
	}
	read := func() (frame, error) {
		var f frame
		_, buf, err := c.Read(sctx)
		if err != nil {
			switch websocket.CloseStatus(err) {
			case 4004:
				return f, errFatal{"Discord refused the bot token: copy it again from the Developer Portal (Bot → Reset Token)"}
			case 4010, 4011, 4013, 4014:
				return f, errFatal{fmt.Sprintf("Discord closed the connection (code %d); check the bot's settings in the Developer Portal", websocket.CloseStatus(err))}
			case 4007, 4009:
				*res = resume{} // the session can't be resumed: start fresh
			}
			return f, err
		}
		if err := json.Unmarshal(buf, &f); err != nil {
			return f, err
		}
		return f, nil
	}
	hello, err := read()
	if err != nil {
		return err
	}
	var h struct {
		Interval int64 `json:"heartbeat_interval"`
	}
	if hello.Op != 10 || json.Unmarshal(hello.D, &h) != nil || h.Interval <= 0 {
		return errors.New("unexpected first message from Discord")
	}
	var seqMu sync.Mutex
	acked := true
	go func() {
		t := time.NewTicker(time.Duration(h.Interval) * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-sctx.Done():
				return
			case <-t.C:
				seqMu.Lock()
				if !acked { // no answer to the last heartbeat: the connection is dead
					seqMu.Unlock()
					c.Close(4000, "heartbeat timeout")
					cancel()
					return
				}
				acked = false
				var s any
				if res.seq > 0 {
					s = res.seq
				}
				seqMu.Unlock()
				_ = send(1, s)
			}
		}
	}()
	if res.session != "" {
		err = send(6, map[string]any{"token": b.Cfg.Token, "session_id": res.session, "seq": res.seq})
	} else {
		err = send(2, map[string]any{"token": b.Cfg.Token, "intents": 1, // GUILDS only: slash commands need nothing more
			"properties": map[string]string{"os": "windows", "browser": "palsforge-warden", "device": "palsforge-warden"}})
	}
	if err != nil {
		return err
	}
	for {
		f, err := read()
		if err != nil {
			return err
		}
		if f.S != nil {
			seqMu.Lock()
			res.seq = *f.S
			seqMu.Unlock()
		}
		switch f.Op {
		case 11:
			seqMu.Lock()
			acked = true
			seqMu.Unlock()
		case 1:
			_ = send(1, res.seq)
		case 7:
			return errors.New("Discord asked the bot to reconnect")
		case 9:
			var ok bool
			_ = json.Unmarshal(f.D, &ok)
			if !ok {
				*res = resume{}
			}
			time.Sleep(2 * time.Second)
			return errors.New("Discord ended the session")
		case 0:
			switch f.T {
			case "READY":
				var r struct {
					Session string `json:"session_id"`
					Resume  string `json:"resume_gateway_url"`
					App     struct {
						ID string `json:"id"`
					} `json:"application"`
					User struct {
						Username string `json:"username"`
					} `json:"user"`
				}
				if json.Unmarshal(f.D, &r) == nil {
					res.session, res.url = r.Session, r.Resume
					b.mu.Lock()
					b.appID = r.App.ID
					b.mu.Unlock()
					if err := b.register(ctx); err != nil {
						b.setState("connected, but the commands couldn't be added: " + err.Error())
						b.logf("registering commands: %v", err)
					} else {
						b.setState("connected as " + r.User.Username)
						b.logf("connected as %s", r.User.Username)
					}
				}
			case "RESUMED":
				b.setState(strings.Replace(b.State(), "reconnecting", "connected", 1))
			case "INTERACTION_CREATE":
				go b.interaction(ctx, f.D)
			}
		}
	}
}

// rest calls Discord's REST API with the bot token.
func (b *Bot) rest(ctx context.Context, method, path string, body, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rd = strings.NewReader(string(buf))
	}
	req, err := http.NewRequestWithContext(ctx, method, b.API+path, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bot "+b.Cfg.Token)
	req.Header.Set("User-Agent", "DiscordBot (https://github.com/rileydj78/palsforge-warden, 1)")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Message == "" {
			e.Message = resp.Status
		}
		return resp.StatusCode, fmt.Errorf("Discord: %s", e.Message)
	}
	if out != nil && len(data) > 0 {
		return resp.StatusCode, json.Unmarshal(data, out)
	}
	return resp.StatusCode, nil
}

func nonce() string {
	b := make([]byte, 9)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
