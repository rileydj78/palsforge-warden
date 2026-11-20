// Package worlds lists a server's worlds and chooses which one it runs. Palworld keeps each world
// in Pal/Saved/SaveGames/0/<32 hex>/ and runs the one named by DedicatedServerName in
// GameUserSettings.ini. Switching only rewrites that line (while the server is stopped); no world
// is ever deleted, so switching back is always possible.
package worlds

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var idRe = regexp.MustCompile(`^[0-9A-F]{32}$`)

// World is one saved world.
type World struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Active   bool      `json:"active"`
	Size     int64     `json:"size"`
	Players  int       `json:"players"`
	Modified time.Time `json:"modified"`
}

func savesDir(serverDir string) string {
	return filepath.Join(serverDir, "Pal", "Saved", "SaveGames", "0")
}

func userSettings(settingsPath string) string {
	return filepath.Join(filepath.Dir(settingsPath), "GameUserSettings.ini")
}

var nameLine = regexp.MustCompile(`(?m)^DedicatedServerName=(.*)$`)

// Active is the world the server runs ("" when the file doesn't say yet).
func Active(settingsPath string) string {
	b, err := os.ReadFile(userSettings(settingsPath))
	if err != nil {
		return ""
	}
	if m := nameLine.FindSubmatch(b); m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	return ""
}

// List returns the worlds, the active one first, then the most recently played.
func List(serverDir, settingsPath, dataDir string) ([]World, error) {
	ents, err := os.ReadDir(savesDir(serverDir))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	labels := loadLabels(dataDir)
	active := Active(settingsPath)
	var out []World
	for _, e := range ents {
		if !e.IsDir() || !idRe.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(savesDir(serverDir), e.Name())
		lv, err := os.Stat(filepath.Join(dir, "Level.sav"))
		if err != nil {
			continue
		}
		w := World{ID: e.Name(), Label: labels[e.Name()], Active: e.Name() == active, Modified: lv.ModTime()}
		filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if i, err := d.Info(); err == nil {
					w.Size += i.Size()
				}
			}
			return nil
		})
		if ps, _ := filepath.Glob(filepath.Join(dir, "Players", "*.sav")); ps != nil {
			w.Players = len(ps)
		}
		out = append(out, w)
	}
	if active != "" && !contains(out, active) { // chosen but not started yet: a fresh world to come
		out = append(out, World{ID: active, Label: labels[active], Active: true})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		return out[i].Modified.After(out[j].Modified)
	})
	return out, nil
}

func contains(ws []World, id string) bool {
	for _, w := range ws {
		if w.ID == id {
			return true
		}
	}
	return false
}

// NewID makes an ID for a fresh world (the game creates it on the next start).
func NewID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

// SetActive points the server at a world. Call only while the server is stopped (the game
// rewrites its settings when it shuts down).
func SetActive(settingsPath, id string) error {
	if !idRe.MatchString(id) {
		return errors.New("not a world ID")
	}
	p := userSettings(settingsPath)
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := string(b)
	switch {
	case nameLine.MatchString(text):
		text = nameLine.ReplaceAllString(text, "DedicatedServerName="+id)
	case strings.Contains(text, "[/Script/Pal.PalGameLocalSettings]"):
		text = strings.Replace(text, "[/Script/Pal.PalGameLocalSettings]", "[/Script/Pal.PalGameLocalSettings]\r\nDedicatedServerName="+id, 1)
	default:
		text = "[/Script/Pal.PalGameLocalSettings]\r\nDedicatedServerName=" + id + "\r\n" + text
	}
	if len(b) > 0 {
		os.WriteFile(p+".warden-"+time.Now().Format("20060102-150405")+".bak", b, 0o644)
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	return os.WriteFile(p, []byte(text), 0o644)
}

func labelsFile(dataDir string) string { return filepath.Join(dataDir, "world-labels.json") }

func loadLabels(dataDir string) map[string]string {
	m := map[string]string{}
	if b, err := os.ReadFile(labelsFile(dataDir)); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

// SetLabel names a world in the panel (e.g. "Season 1").
func SetLabel(dataDir, id, label string) error {
	if !idRe.MatchString(id) {
		return errors.New("not a world ID")
	}
	m := loadLabels(dataDir)
	if label = strings.TrimSpace(label); label == "" {
		delete(m, id)
	} else {
		m[id] = label
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(labelsFile(dataDir), b, 0o600)
}

// Pending: a switch chosen in the panel, applied while the server is stopped.

func pendingFile(dataDir string) string { return filepath.Join(dataDir, "world-pending.txt") }

// Queue chooses the world for the next start.
func Queue(dataDir, id string) error {
	if !idRe.MatchString(id) {
		return errors.New("not a world ID")
	}
	return os.WriteFile(pendingFile(dataDir), []byte(id), 0o600)
}

// Pending is the queued world, if any.
func Pending(dataDir string) string {
	b, _ := os.ReadFile(pendingFile(dataDir))
	return strings.TrimSpace(string(b))
}

// ApplyPending switches to the queued world (call only while stopped).
func ApplyPending(settingsPath, dataDir string) (string, error) {
	id := Pending(dataDir)
	if id == "" {
		return "", nil
	}
	if err := SetActive(settingsPath, id); err != nil {
		return "", err
	}
	os.Remove(pendingFile(dataDir))
	return "world switched to " + id, nil
}

// Cancel drops a queued switch.
func Cancel(dataDir string) { os.Remove(pendingFile(dataDir)) }
