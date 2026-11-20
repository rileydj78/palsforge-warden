package discordbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// command is one slash command: who may use it, what it asks, and how it runs.
type command struct {
	name, desc string
	min        string // RoleNone: anyone in the Discord server
	options    []map[string]any
	confirm    string // non-empty: ask "are you sure?" with this button label first
	slow       bool   // may take minutes: answer "working on it", then edit
	run        func(b *Bot, ctx context.Context, role, who string, args map[string]any) string
}

func opt(kind int, name, desc string, required, auto bool) map[string]any {
	o := map[string]any{"type": kind, "name": name, "description": desc, "required": required}
	if auto {
		o["autocomplete"] = true
	}
	return o
}

const (
	optString  = 3
	optInteger = 4
)

var commands = []command{
	{name: "status", desc: "Is the server up, and who's on?", min: RoleNone, run: (*Bot).cmdStatus},
	{name: "players", desc: "Players online, with level and ping", min: RoleModerator, run: (*Bot).cmdPlayers},
	{name: "announce", desc: "Send a message to everyone in the game", min: RoleModerator,
		options: []map[string]any{opt(optString, "message", "What to say", true, false)},
		run: func(b *Bot, ctx context.Context, role, who string, a map[string]any) string {
			return b.post(ctx, role, who, "/api/action/announce", map[string]any{"message": str(a, "message")}, "Sent to the game.")
		}},
	{name: "kick", desc: "Kick a player (they can come back)", min: RoleModerator,
		options: []map[string]any{opt(optString, "player", "Who", true, true), opt(optString, "reason", "Why (shown to them)", false, false)},
		run: func(b *Bot, ctx context.Context, role, who string, a map[string]any) string {
			id, name := b.player(ctx, role, who, str(a, "player"))
			if id == "" {
				return "I can't find that player online."
			}
			return b.post(ctx, role, who, "/api/action/kick", map[string]any{"userId": id, "name": name, "message": str(a, "reason")}, "Kicked "+name+".")
		}},
	{name: "ban", desc: "Ban a player", min: RoleAdmin, confirm: "Ban",
		options: []map[string]any{opt(optString, "player", "Who", true, true), opt(optString, "reason", "Why (shown to them)", false, false)},
		run: func(b *Bot, ctx context.Context, role, who string, a map[string]any) string {
			id, name := b.player(ctx, role, who, str(a, "player"))
			if id == "" {
				return "I can't find that player online. Bans of offline players are on the panel's Players page."
			}
			return b.post(ctx, role, who, "/api/action/ban", map[string]any{"userId": id, "name": name, "message": str(a, "reason")}, "Banned "+name+".")
		}},
	{name: "save", desc: "Save the world now", min: RoleAdmin,
		run: func(b *Bot, ctx context.Context, role, who string, _ map[string]any) string {
			return b.post(ctx, role, who, "/api/action/save", map[string]any{}, "World saved.")
		}},
	{name: "backup", desc: "Make a backup now", min: RoleAdmin, slow: true,
		run: func(b *Bot, ctx context.Context, role, who string, _ map[string]any) string {
			return b.post(ctx, role, who, "/api/backups/now", map[string]any{}, "Backup made.")
		}},
	{name: "restart", desc: "Restart the server, warning players first", min: RoleAdmin, confirm: "Restart",
		options: []map[string]any{{"type": optInteger, "name": "minutes", "description": "Warn players this many minutes ahead (0–60, default 5)", "required": false, "min_value": 0, "max_value": 60}},
		run: func(b *Bot, ctx context.Context, role, who string, a map[string]any) string {
			m := 5
			if v, ok := a["minutes"].(float64); ok {
				m = int(v)
			}
			ok := "Restarting now."
			if m > 0 {
				ok = fmt.Sprintf("Restart in %d minute(s); players are being warned.", m)
			}
			return b.post(ctx, role, who, "/api/server/restart", map[string]any{"minutes": m}, ok)
		}},
	{name: "start", desc: "Start the server", min: RoleAdmin, slow: true,
		run: func(b *Bot, ctx context.Context, role, who string, _ map[string]any) string {
			return b.post(ctx, role, who, "/api/server/start", map[string]any{}, "Server started.")
		}},
	{name: "stop", desc: "Stop the server (players get 10 seconds' warning)", min: RoleAdmin, confirm: "Stop the server", slow: true,
		run: func(b *Bot, ctx context.Context, role, who string, _ map[string]any) string {
			return b.post(ctx, role, who, "/api/server/stop", map[string]any{}, "Server stopped.")
		}},
}

func find(name string) *command {
	for i := range commands {
		if commands[i].name == name {
			return &commands[i]
		}
	}
	return nil
}

// register replaces the server's commands with Warden's.
func (b *Bot) register(ctx context.Context) error {
	var defs []map[string]any
	for _, c := range commands {
		d := map[string]any{"name": c.name, "description": c.desc, "type": 1, "contexts": []int{0}}
		if len(c.options) > 0 {
			d["options"] = c.options
		}
		defs = append(defs, d)
	}
	b.mu.Lock()
	app := b.appID
	b.mu.Unlock()
	_, err := b.rest(ctx, "PUT", "/applications/"+app+"/guilds/"+b.Cfg.Guild+"/commands", defs, nil)
	return err
}

type interaction struct {
	ID      string `json:"id"`
	Type    int    `json:"type"`
	Token   string `json:"token"`
	Guild   string `json:"guild_id"`
	Channel string `json:"channel_id"`
	Member  *struct {
		Roles []string `json:"roles"`
		User  struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	} `json:"member"`
	Data struct {
		Name     string `json:"name"`
		CustomID string `json:"custom_id"`
		Options  []struct {
			Name    string `json:"name"`
			Value   any    `json:"value"`
			Focused bool   `json:"focused"`
		} `json:"options"`
	} `json:"data"`
}

const ephemeral = 64

func (b *Bot) interaction(ctx context.Context, raw json.RawMessage) {
	var in interaction
	if json.Unmarshal(raw, &in) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	reply := func(kind int, data map[string]any) {
		if _, err := b.rest(ctx, "POST", "/interactions/"+in.ID+"/"+in.Token+"/callback", map[string]any{"type": kind, "data": data}, nil); err != nil {
			b.logf("answering: %v", err)
		}
	}
	say := func(text string) {
		reply(4, map[string]any{"content": text, "flags": ephemeral, "allowed_mentions": map[string]any{"parse": []string{}}})
	}
	if in.Guild != b.Cfg.Guild || in.Member == nil { // DMs and other servers: nothing
		if in.Type == 2 {
			say("This bot only answers in its own Discord server.")
		}
		return
	}
	if b.Cfg.Channel != "" && in.Channel != b.Cfg.Channel && in.Type != 4 {
		say("Use Warden's commands in <#" + b.Cfg.Channel + ">.")
		return
	}
	role := b.Cfg.RoleFor(in.Member.Roles)
	who := "discord:" + in.Member.User.Username
	args := map[string]any{}
	for _, o := range in.Data.Options {
		args[o.Name] = o.Value
	}
	switch in.Type {
	case 4: // autocomplete: the player list
		if role == RoleNone {
			reply(8, map[string]any{"choices": []any{}})
			return
		}
		typed := ""
		for _, o := range in.Data.Options {
			if o.Focused {
				typed, _ = o.Value.(string)
			}
		}
		var choices []map[string]string
		for _, p := range b.online(ctx, role, who) {
			if typed == "" || strings.Contains(strings.ToLower(p.Name), strings.ToLower(typed)) {
				choices = append(choices, map[string]string{"name": clip(p.Name, 100), "value": p.UserID})
			}
			if len(choices) == 25 {
				break
			}
		}
		reply(8, map[string]any{"choices": choices})
	case 2: // a command
		c := find(in.Data.Name)
		if c == nil {
			say("I don't know that command any more; it may take a minute for Discord to catch up.")
			return
		}
		if !allows(role, c.min) {
			say("Your Discord roles don't allow /" + c.name + ". Ask an admin to give you Warden's " + c.min + " role.")
			return
		}
		if c.confirm != "" {
			id := nonce()
			b.mu.Lock()
			for k, p := range b.pending {
				if time.Now().After(p.expires) {
					delete(b.pending, k)
				}
			}
			b.pending[id] = pendingAction{user: in.Member.User.ID, command: c.name, args: args, expires: time.Now().Add(2 * time.Minute)}
			b.mu.Unlock()
			reply(4, map[string]any{"content": "Are you sure? " + describe(c.name, args), "flags": ephemeral, "allowed_mentions": map[string]any{"parse": []string{}},
				"components": []any{map[string]any{"type": 1, "components": []any{
					map[string]any{"type": 2, "style": 4, "label": c.confirm, "custom_id": "yes:" + id},
					map[string]any{"type": 2, "style": 2, "label": "Cancel", "custom_id": "no:" + id},
				}}}})
			return
		}
		b.execute(ctx, in, c, role, who, args, false)
	case 3: // a button on a confirmation
		yes, id, _ := strings.Cut(in.Data.CustomID, ":")
		b.mu.Lock()
		p, ok := b.pending[id]
		if ok && p.user == in.Member.User.ID { // someone else's press leaves the question open
			delete(b.pending, id)
		}
		b.mu.Unlock()
		update := func(text string) {
			reply(7, map[string]any{"content": text, "components": []any{}, "allowed_mentions": map[string]any{"parse": []string{}}})
		}
		switch {
		case !ok || time.Now().After(p.expires):
			update("That question expired; run the command again.")
		case p.user != in.Member.User.ID:
			say("Only the person who asked can answer that.")
		case yes != "yes":
			update("Cancelled.")
		default:
			c := find(p.command)
			if c == nil || !allows(role, c.min) { // roles checked again: they may have changed
				update("Your Discord roles no longer allow that.")
				return
			}
			b.execute(ctx, in, c, role, who, p.args, true)
		}
	}
}

// execute runs a command and answers; slow ones say "working on it" first and edit after.
func (b *Bot) execute(ctx context.Context, in interaction, c *command, role, who string, args map[string]any, fromButton bool) {
	public := c.name == "status"
	flags := ephemeral
	if public {
		flags = 0
	}
	if c.slow {
		kind, data := 5, map[string]any{"flags": flags}
		if fromButton {
			kind, data = 7, map[string]any{"content": "Working on it…", "components": []any{}}
		}
		if _, err := b.rest(ctx, "POST", "/interactions/"+in.ID+"/"+in.Token+"/callback", map[string]any{"type": kind, "data": data}, nil); err != nil {
			b.logf("answering: %v", err)
			return
		}
		text := c.run(b, ctx, role, who, args)
		b.mu.Lock()
		app := b.appID
		b.mu.Unlock()
		if _, err := b.rest(ctx, "PATCH", "/webhooks/"+app+"/"+in.Token+"/messages/@original", map[string]any{"content": clip(text, 1900), "allowed_mentions": map[string]any{"parse": []string{}}}, nil); err != nil {
			b.logf("answering: %v", err)
		}
		return
	}
	text := clip(c.run(b, ctx, role, who, args), 1900)
	kind, data := 4, map[string]any{"content": text, "flags": flags, "allowed_mentions": map[string]any{"parse": []string{}}}
	if fromButton {
		kind, data = 7, map[string]any{"content": text, "components": []any{}, "allowed_mentions": map[string]any{"parse": []string{}}}
	}
	if _, err := b.rest(ctx, "POST", "/interactions/"+in.ID+"/"+in.Token+"/callback", map[string]any{"type": kind, "data": data}, nil); err != nil {
		b.logf("answering: %v", err)
	}
}

func allows(role, min string) bool {
	rank := map[string]int{RoleNone: 0, RoleModerator: 1, RoleAdmin: 2}
	return rank[role] >= rank[min]
}

func describe(name string, a map[string]any) string {
	switch name {
	case "ban":
		return "Ban the selected player" + reason(a) + "."
	case "restart":
		m := 5
		if v, ok := a["minutes"].(float64); ok {
			m = int(v)
		}
		return fmt.Sprintf("Restart the server with %d minute(s) of warning.", m)
	case "stop":
		return "Stop the server. Players are disconnected after a short warning."
	}
	return "/" + name
}

func reason(a map[string]any) string {
	if r := str(a, "reason"); r != "" {
		return " (reason: " + clip(r, 100) + ")"
	}
	return ""
}

func str(a map[string]any, k string) string {
	s, _ := a[k].(string)
	return strings.TrimSpace(s)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// post calls an action and turns the answer into a sentence.
func (b *Bot) post(ctx context.Context, role, who, path string, body map[string]any, ok string) string {
	code, data := b.Call(ctx, role, who, "POST", path, body)
	if code >= 200 && code < 300 {
		return ok
	}
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(data, &e)
	if e.Error == "" {
		e.Error = fmt.Sprintf("Warden answered %d", code)
	}
	return "Couldn't: " + e.Error
}

type onlinePlayer struct {
	Name   string  `json:"name"`
	Level  int     `json:"level"`
	Ping   float64 `json:"ping"`
	UserID string  `json:"userId"`
}

type statusView struct {
	Online  bool `json:"online"`
	Paused  bool `json:"paused"`
	Stopped bool `json:"stopped"`
	Server  *struct {
		Name    string `json:"servername"`
		Version string `json:"version"`
	} `json:"server"`
	Metrics *struct {
		FPS  float64 `json:"serverfps"`
		Max  int     `json:"maxplayernum"`
		Days int     `json:"days"`
	} `json:"metrics"`
	Players []onlinePlayer `json:"players"`
}

func (b *Bot) status(ctx context.Context, role, who string) (*statusView, error) {
	if role == RoleNone { // /status for everyone: read it as a moderator, show only the public part
		role = RoleModerator
	}
	code, data := b.Call(ctx, role, who, "GET", "/api/status", nil)
	if code != 200 {
		return nil, fmt.Errorf("Warden answered %d", code)
	}
	var s statusView
	return &s, json.Unmarshal(data, &s)
}

func (b *Bot) online(ctx context.Context, role, who string) []onlinePlayer {
	s, err := b.status(ctx, role, who)
	if err != nil {
		return nil
	}
	return s.Players
}

// player resolves what the member picked (a user ID from autocomplete, or a typed name).
func (b *Bot) player(ctx context.Context, role, who, v string) (id, name string) {
	ps := b.online(ctx, role, who)
	for _, p := range ps {
		if p.UserID == v {
			return p.UserID, p.Name
		}
	}
	for _, p := range ps {
		if strings.EqualFold(p.Name, v) {
			return p.UserID, p.Name
		}
	}
	return "", ""
}

func (b *Bot) cmdStatus(ctx context.Context, role, who string, _ map[string]any) string {
	s, err := b.status(ctx, role, who)
	if err != nil {
		return "Warden couldn't check the server just now."
	}
	switch {
	case s.Paused:
		return "💤 The server is paused to save power; it wakes when someone joins."
	case s.Stopped:
		return "⏹ The server is stopped."
	case !s.Online:
		return "🔴 The server isn't answering."
	}
	name, ver := "The server", ""
	if s.Server != nil {
		name, ver = s.Server.Name, s.Server.Version
	}
	line := fmt.Sprintf("🟢 **%s** is up", name)
	if s.Metrics != nil {
		line += fmt.Sprintf(": %d/%d players, day %d", len(s.Players), s.Metrics.Max, s.Metrics.Days)
	}
	if ver != "" {
		line += " (game " + ver + ")"
	}
	return line
}

func (b *Bot) cmdPlayers(ctx context.Context, role, who string, _ map[string]any) string {
	s, err := b.status(ctx, role, who)
	if err != nil {
		return "Warden couldn't check the server just now."
	}
	if len(s.Players) == 0 {
		return "Nobody is online."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d online:\n", len(s.Players))
	for _, p := range s.Players {
		fmt.Fprintf(&sb, "• %s (level %d, %.0f ms)\n", p.Name, p.Level, p.Ping)
	}
	return sb.String()
}
