// Package notify tells the admins about things that matter, through a Discord webhook and/or
// ntfy (phone push). Nothing is sent unless the admin set one up; messages never include player
// IP addresses or passwords.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Config is the "notify" part of warden.json.
type Config struct {
	DiscordWebhook string `json:"discordWebhook"` // https://discord.com/api/webhooks/...
	NtfyURL        string `json:"ntfyUrl"`        // https://ntfy.sh/<topic> or your own server
	NtfyToken      string `json:"ntfyToken"`      // optional access token
	RoutineOK      bool   `json:"routineOk"`      // also report runs that went fine (default: problems only)
}

// Check validates the addresses.
func (c Config) Check() error {
	if c.DiscordWebhook != "" && !strings.HasPrefix(c.DiscordWebhook, "https://discord.com/api/webhooks/") && !strings.HasPrefix(c.DiscordWebhook, "https://discordapp.com/api/webhooks/") {
		return fmt.Errorf("notify.discordWebhook must be a Discord webhook address (https://discord.com/api/webhooks/...)")
	}
	if c.NtfyURL != "" {
		u, err := url.Parse(c.NtfyURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || strings.Trim(u.Path, "/") == "" {
			return fmt.Errorf("notify.ntfyUrl must look like https://ntfy.sh/your-topic")
		}
	}
	return nil
}

// Level says how loud a message is.
type Level int

const (
	Info Level = iota
	Warn
	Urgent
)

// Notifier sends messages, rate-limited so a crash loop can't flood a channel.
type Notifier struct {
	Cfg    Config
	Server string // server name, for the title
	HTTP   *http.Client
	mu     sync.Mutex
	recent map[string]time.Time
}

// New returns a Notifier (a no-op when nothing is configured).
func New(cfg Config, server string) *Notifier {
	return &Notifier{Cfg: cfg, Server: server, HTTP: &http.Client{Timeout: 15 * time.Second}, recent: map[string]time.Time{}}
}

// Enabled says whether anything is set up.
func (n *Notifier) Enabled() bool {
	return n != nil && (n.Cfg.DiscordWebhook != "" || n.Cfg.NtfyURL != "")
}

// Send delivers a message to every configured channel. The same key is sent at most once in
// 10 minutes (an empty key is never limited).
func (n *Notifier) Send(ctx context.Context, lvl Level, key, title, body string) {
	if !n.Enabled() {
		return
	}
	if key != "" {
		n.mu.Lock()
		if t, ok := n.recent[key]; ok && time.Since(t) < 10*time.Minute {
			n.mu.Unlock()
			return
		}
		n.recent[key] = time.Now()
		n.mu.Unlock()
	}
	full := title
	if n.Server != "" {
		full = n.Server + ": " + title
	}
	if n.Cfg.DiscordWebhook != "" {
		icon := map[Level]string{Info: "✅", Warn: "⚠️", Urgent: "🚨"}[lvl]
		msg := map[string]any{"username": "Warden", "content": fmt.Sprintf("%s **%s**\n%s", icon, full, body), "allowed_mentions": map[string]any{"parse": []string{}}}
		b, _ := json.Marshal(msg)
		n.post(ctx, n.Cfg.DiscordWebhook, b, map[string]string{"Content-Type": "application/json"})
	}
	if n.Cfg.NtfyURL != "" {
		h := map[string]string{"Title": asciiHeader(full), "Priority": map[Level]string{Info: "default", Warn: "high", Urgent: "urgent"}[lvl], "Tags": map[Level]string{Info: "white_check_mark", Warn: "warning", Urgent: "rotating_light"}[lvl]}
		if n.Cfg.NtfyToken != "" {
			h["Authorization"] = "Bearer " + n.Cfg.NtfyToken
		}
		n.post(ctx, n.Cfg.NtfyURL, []byte(body), h)
	}
}

func (n *Notifier) post(ctx context.Context, u string, body []byte, h map[string]string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return
	}
	for k, v := range h {
		req.Header.Set(k, v)
	}
	resp, err := n.HTTP.Do(req)
	if err != nil {
		log.Printf("notify: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("notify: %s answered %s", req.URL.Host, resp.Status)
	}
}

// asciiHeader keeps HTTP header values plain ASCII (ntfy reads titles from a header).
func asciiHeader(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r >= 32 && r < 127 {
			b = append(b, byte(r))
		}
	}
	return string(b)
}
