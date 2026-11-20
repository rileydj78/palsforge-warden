package gamesettings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Palworld writes PalWorldSettings.ini back from memory when it shuts down, so a change written
// while the server runs is lost at the next stop. Changes are therefore kept here and written
// only while the server is stopped (by the restart that applies them, or the nightly routine).

func pendingPath(dataDir string) string { return filepath.Join(dataDir, "game-settings-pending.json") }

// LoadPending returns the changes waiting for the next stop.
func LoadPending(dataDir string) map[string]string {
	out := map[string]string{}
	if b, err := os.ReadFile(pendingPath(dataDir)); err == nil {
		json.Unmarshal(b, &out)
	}
	return out
}

// AddPending merges new values into the waiting changes.
func AddPending(dataDir string, values map[string]string) error {
	p := LoadPending(dataDir)
	for k, v := range values {
		p[k] = v
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(pendingPath(dataDir), b, 0o600)
}

// ClearPending forgets the waiting changes.
func ClearPending(dataDir string) { os.Remove(pendingPath(dataDir)) }

// ApplyPending writes the waiting changes into the settings file. Call it only while the server is
// stopped. A change that can't be applied (an option the file doesn't have, a bad value) is set
// aside in game-settings-rejected.json and reported, never left to block the others.
func ApplyPending(iniPath, dataDir string) (string, error) {
	p := LoadPending(dataDir)
	if len(p) == 0 {
		return "", nil
	}
	f, err := Read(iniPath)
	if err != nil {
		return "", err
	}
	good, rejected := map[string]string{}, map[string]string{}
	var why []string
	for k, v := range p {
		if _, err := f.Plan(map[string]string{k: v}); err != nil {
			rejected[k] = v
			why = append(why, err.Error())
			continue
		}
		good[k] = v
	}
	ch, _ := f.Plan(good)
	if len(ch) > 0 {
		if _, err := f.Apply(ch); err != nil {
			return "", err // nothing written: the waiting changes stay for the next stop
		}
	}
	ClearPending(dataDir)
	if len(rejected) > 0 {
		b, _ := json.MarshalIndent(rejected, "", "  ")
		os.WriteFile(filepath.Join(dataDir, "game-settings-rejected.json"), b, 0o600)
	}
	var names []string
	for _, c := range ch {
		names = append(names, c.Key)
	}
	sort.Strings(names)
	sum := fmt.Sprintf("game settings: %d changed (%s)", len(ch), strings.Join(names, ", "))
	if len(why) > 0 {
		return sum, fmt.Errorf("%d waiting change(s) couldn't be applied and were set aside: %s", len(why), strings.Join(why, "; "))
	}
	return sum, nil
}

// Effective is the file's values with the waiting changes laid over them: what the server will
// run with after the next stop.
func Effective(iniPath, dataDir string) map[string]string {
	out := map[string]string{}
	if f, err := Read(iniPath); err == nil {
		for k, v := range f.Values {
			out[k] = v
		}
	}
	for k, v := range LoadPending(dataDir) {
		out[k] = v
	}
	return out
}
