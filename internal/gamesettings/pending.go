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
// stopped. It returns a short summary ("" when nothing was waiting).
func ApplyPending(iniPath, dataDir string) (string, error) {
	p := LoadPending(dataDir)
	if len(p) == 0 {
		return "", nil
	}
	f, err := Read(iniPath)
	if err != nil {
		return "", err
	}
	ch, err := f.Plan(p)
	if err != nil {
		return "", fmt.Errorf("waiting settings changes can't be applied: %w", err)
	}
	if len(ch) > 0 {
		if _, err := f.Apply(ch); err != nil {
			return "", err
		}
	}
	ClearPending(dataDir)
	var names []string
	for _, c := range ch {
		names = append(names, c.Key)
	}
	sort.Strings(names)
	return fmt.Sprintf("game settings: %d changed (%s)", len(ch), strings.Join(names, ", ")), nil
}
