package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StatusInfo is what the live Discord status message shows.
type StatusInfo struct {
	Online      bool
	Name        string
	Description string
	Players     []string
	Max         int
	FPS         float64
	UptimeSecs  int64
	Day         int
}

// LiveStatus keeps one Discord message up to date with the server's state: posted once through
// the webhook, then edited in place (webhooks may edit their own messages; no bot needed).
type LiveStatus struct {
	Webhook string
	DataDir string
	HTTP    *http.Client
	last    string
}

func (l *LiveStatus) idPath() string { return filepath.Join(l.DataDir, "discord-status-message.txt") }

// Update posts or edits the message. It does nothing when the content hasn't changed.
func (l *LiveStatus) Update(ctx context.Context, s StatusInfo) error {
	if l.Webhook == "" {
		return nil
	}
	if l.HTTP == nil {
		l.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	color, state := 0xD14B3C, "🔴 Offline"
	if s.Online {
		color, state = 0x48C78E, "🟢 Online"
	}
	players := "Nobody right now. Be the first."
	if len(s.Players) > 0 {
		players = strings.Join(s.Players, ", ")
		if len(players) > 900 {
			players = players[:900] + "…"
		}
	}
	fields := []map[string]any{
		{"name": "Status", "value": state, "inline": true},
		{"name": "Players", "value": fmt.Sprintf("%d / %d", len(s.Players), s.Max), "inline": true},
	}
	if s.Online {
		up := time.Duration(s.UptimeSecs) * time.Second
		fields = append(fields,
			map[string]any{"name": "Server FPS", "value": fmt.Sprintf("%.0f", s.FPS), "inline": true},
			map[string]any{"name": "Up for", "value": fmt.Sprintf("%dh %dm", int(up.Hours()), int(up.Minutes())%60), "inline": true},
			map[string]any{"name": "World day", "value": fmt.Sprint(s.Day), "inline": true})
	}
	fields = append(fields, map[string]any{"name": "Online now", "value": players})
	body := map[string]any{
		"username":         "Warden",
		"allowed_mentions": map[string]any{"parse": []string{}},
		"embeds": []map[string]any{{
			"title": s.Name, "description": s.Description, "color": color, "fields": fields,
			"footer": map[string]any{"text": "Updated"}, "timestamp": time.Now().UTC().Format(time.RFC3339),
		}},
	}
	key := fmt.Sprint(s.Online, s.Name, s.Description, s.Players, s.Max, int(s.FPS/5), s.UptimeSecs/600, s.Day)
	if key == l.last {
		return nil
	}
	b, _ := json.Marshal(body)
	id, _ := os.ReadFile(l.idPath())
	if msgID := strings.TrimSpace(string(id)); msgID != "" {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPatch, l.Webhook+"/messages/"+msgID, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		resp, err := l.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("couldn't reach Discord")
		}
		resp.Body.Close()
		if resp.StatusCode < 300 {
			l.last = key
			return nil
		}
		if resp.StatusCode != http.StatusNotFound { // deleted message: post a new one below
			return fmt.Errorf("Discord answered %s", resp.Status)
		}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, l.Webhook+"?wait=true", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach Discord")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Discord answered %s", resp.Status)
	}
	var m struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&m)
	if m.ID != "" {
		os.WriteFile(l.idPath(), []byte(m.ID), 0o600)
	}
	l.last = key
	return nil
}
