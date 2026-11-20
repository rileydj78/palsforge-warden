// Package worlds lists a server's worlds and chooses which one it runs. Palworld keeps each world
// in Pal/Saved/SaveGames/0/<32 hex>/ and runs the one named by DedicatedServerName in
// GameUserSettings.ini. Switching only rewrites that line (while the server is stopped); no world
// is ever deleted, so switching back is always possible.
package worlds

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
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

// OptionFile is the active world's WorldOption.sav, if it has one. The game prefers it to
// PalWorldSettings.ini for almost every setting (a co-op world uploaded to a server brings one
// along), so changes made in the settings file silently don't apply while it exists.
func OptionFile(serverDir, settingsPath string) string {
	id := Active(settingsPath)
	if id == "" {
		return ""
	}
	p := filepath.Join(savesDir(serverDir), id, "WorldOption.sav")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func optionPending(dataDir string) string { return filepath.Join(dataDir, "worldoption-setaside.txt") }

// QueueSetAsideOption asks for the active world's WorldOption.sav to be set aside at the next stop.
func QueueSetAsideOption(dataDir string) error {
	return os.WriteFile(optionPending(dataDir), []byte("yes"), 0o600)
}

// SetAsideOptionQueued says whether that's waiting.
func SetAsideOptionQueued(dataDir string) bool {
	_, err := os.Stat(optionPending(dataDir))
	return err == nil
}

// ApplySetAsideOption renames the active world's WorldOption.sav (kept beside it, so it can be put
// back) so the settings file applies. Call only while the server is stopped.
func ApplySetAsideOption(serverDir, settingsPath, dataDir string) (string, error) {
	if !SetAsideOptionQueued(dataDir) {
		return "", nil
	}
	os.Remove(optionPending(dataDir))
	p := OptionFile(serverDir, settingsPath)
	if p == "" {
		return "", nil
	}
	to := p + ".warden-" + time.Now().Format("20060102-150405") + ".bak"
	if err := os.Rename(p, to); err != nil {
		return "", err
	}
	return "WorldOption.sav set aside (kept as " + filepath.Base(to) + "); the settings file applies now", nil
}

// MaxImport is the largest world zip Warden takes (co-op worlds are usually well under 500 MB).
const MaxImport = 2 << 30

// Import unpacks a world uploaded from co-op (a zip of the world folder: Level.sav, LevelMeta.sav,
// Players/, maybe WorldOption.sav) as a new world beside the others and returns its ID. Its
// WorldOption.sav is set aside (it would override the server's settings). The world isn't
// switched to: SwitchWorld does that, with a backup first. Nothing outside the new folder is
// written.
func Import(serverDir, zipPath string) (id string, notes []string, err error) {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", nil, errors.New("that isn't a .zip file")
	}
	defer z.Close()
	// Find Level.sav: at the top or inside one folder (the world folder itself, zipped).
	prefix, found := "", false
	for _, f := range z.File {
		n := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.EqualFold(path.Base(n), "Level.sav") && strings.Count(strings.Trim(n, "/"), "/") <= 1 {
			prefix, found = strings.TrimSuffix(n, path.Base(n)), true
			break
		}
	}
	if !found {
		return "", nil, errors.New("there's no Level.sav in it: zip the world's folder (the one with Level.sav, LevelMeta.sav and Players)")
	}
	id = strings.ToUpper(strings.Trim(path.Base(strings.TrimSuffix(prefix, "/")), "/"))
	if !idRe.MatchString(id) {
		id = NewID()
	}
	dst := filepath.Join(savesDir(serverDir), id)
	if _, err := os.Stat(dst); err == nil {
		id = NewID() // never overwrite a world that's already here
		dst = filepath.Join(savesDir(serverDir), id)
		notes = append(notes, "a world with that ID is already on the server, so this one got a new ID")
	}
	var total int64
	for _, f := range z.File {
		n := strings.ReplaceAll(f.Name, "\\", "/")
		if !strings.HasPrefix(n, prefix) || strings.HasSuffix(n, "/") {
			continue
		}
		rel := strings.TrimPrefix(n, prefix)
		if rel == "" || strings.Contains(rel, "..") || path.IsAbs(rel) || strings.Contains(rel, ":") {
			continue // nothing escapes the world folder
		}
		if total += int64(f.UncompressedSize64); total > 8<<30 {
			os.RemoveAll(dst)
			return "", nil, errors.New("that world unpacks to more than 8 GB; it doesn't look like a Palworld world")
		}
		to := filepath.Join(dst, filepath.FromSlash(rel))
		if strings.EqualFold(rel, "WorldOption.sav") {
			to += ".from-coop.bak"
			notes = append(notes, "its WorldOption.sav was set aside, so the server's own settings apply")
		}
		if err := writeZipFile(f, to); err != nil {
			os.RemoveAll(dst)
			return "", nil, err
		}
	}
	if ps, _ := filepath.Glob(filepath.Join(dst, "Players", "00000000000000000000000000000001.sav")); len(ps) > 0 {
		notes = append(notes, "it has a co-op host's character: on a dedicated server the host starts a new character (everyone else keeps theirs)")
	}
	return id, notes, nil
}

func writeZipFile(f *zip.File, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(r, int64(f.UncompressedSize64)+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
