// Package events runs weekly game events: for example a double-XP weekend from Friday 18:00 to
// Sunday 22:00. At the start Warden remembers the normal values, queues the event's settings and
// restarts the server with warnings that announce it; at the end it puts the normal values back.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Event is one weekly event, written in Settings as:
//
//	Double XP weekend | Fri 18:00 | Sun 22:00 | ExpRate=2, PalCaptureRate=1.5
type Event struct {
	Name     string            `json:"name"`
	Start    string            `json:"start"` // "Fri 18:00"
	End      string            `json:"end"`   // "Sun 22:00"
	Settings map[string]string `json:"settings"`
}

var days = map[string]time.Weekday{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}

// at turns "Fri 18:00" into minutes since Sunday 00:00.
func at(s string) (int, error) {
	d, hm, ok := strings.Cut(strings.TrimSpace(strings.ToLower(s)), " ")
	wd, okd := days[strings.TrimSpace(d)[:min(3, len(strings.TrimSpace(d)))]]
	if !ok || !okd {
		return 0, fmt.Errorf("%q should look like \"Fri 18:00\"", s)
	}
	t, err := time.Parse("15:04", strings.TrimSpace(hm))
	if err != nil {
		return 0, fmt.Errorf("%q should look like \"Fri 18:00\"", s)
	}
	return int(wd)*1440 + t.Hour()*60 + t.Minute(), nil
}

// Active says whether the event is on at time now (windows may wrap past Saturday night).
func (e Event) Active(now time.Time) bool {
	a, err1 := at(e.Start)
	b, err2 := at(e.End)
	if err1 != nil || err2 != nil || a == b {
		return false
	}
	m := int(now.Weekday())*1440 + now.Hour()*60 + now.Minute()
	if a < b {
		return m >= a && m < b
	}
	return m >= a || m < b
}

// Check validates an event.
func (e Event) Check() error {
	if strings.TrimSpace(e.Name) == "" {
		return errors.New("an event needs a name")
	}
	if _, err := at(e.Start); err != nil {
		return err
	}
	if _, err := at(e.End); err != nil {
		return err
	}
	if len(e.Settings) == 0 {
		return fmt.Errorf("%s changes no settings", e.Name)
	}
	return nil
}

// Parse reads events written one per line: "Name | Fri 18:00 | Sun 22:00 | Key=Value, Key=Value".
func Parse(text string) ([]Event, error) {
	var out []Event
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 4 {
			return nil, fmt.Errorf("line %d: write it as  Name | Fri 18:00 | Sun 22:00 | ExpRate=2, PalCaptureRate=1.5", n+1)
		}
		e := Event{Name: strings.TrimSpace(parts[0]), Start: strings.TrimSpace(parts[1]), End: strings.TrimSpace(parts[2]), Settings: map[string]string{}}
		for _, kv := range strings.Split(parts[3], ",") {
			k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
			if !ok {
				return nil, fmt.Errorf("line %d: settings look like ExpRate=2", n+1)
			}
			e.Settings[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
		if err := e.Check(); err != nil {
			return nil, fmt.Errorf("line %d: %w", n+1, err)
		}
		out = append(out, e)
	}
	return out, nil
}

// State remembers which event is on and the normal values it replaced (survives restarts).
type State struct {
	Active string            `json:"active"`
	Normal map[string]string `json:"normal"`
}

func statePath(dataDir string) string { return filepath.Join(dataDir, "events-state.json") }

// Load reads the state.
func Load(dataDir string) State {
	var s State
	if b, err := os.ReadFile(statePath(dataDir)); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

// Save writes the state.
func Save(dataDir string, s State) error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(statePath(dataDir), b, 0o600)
}

// Want returns the event that should be on now ("" for none): the first listed that's active.
func Want(evs []Event, now time.Time) *Event {
	for i := range evs {
		if evs[i].Active(now) {
			return &evs[i]
		}
	}
	return nil
}
