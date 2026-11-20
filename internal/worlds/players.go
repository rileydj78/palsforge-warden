package worlds

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PlayerSave is one player's save file in a world: the game creates it when they first join.
type PlayerSave struct {
	UID     string    // the file name: the game's player ID (32 hex digits)
	Created time.Time // zero where the system doesn't record creation times
}

// PlayerSaves lists the players the active world has saves for.
func PlayerSaves(serverDir, settingsPath string) (world string, out []PlayerSave) {
	world = Active(settingsPath)
	if world == "" {
		return "", nil
	}
	des, err := os.ReadDir(filepath.Join(savesDir(serverDir), world, "Players"))
	if err != nil {
		return world, nil
	}
	for _, d := range des {
		name := d.Name()
		if d.IsDir() || !strings.EqualFold(filepath.Ext(name), ".sav") {
			continue
		}
		uid := strings.TrimSuffix(name, filepath.Ext(name))
		if len(uid) != 32 || strings.Contains(uid, "_") {
			continue // e.g. *_dps.sav (dimensional Pal storage), not a player
		}
		out = append(out, PlayerSave{UID: strings.ToUpper(uid), Created: created(filepath.Join(savesDir(serverDir), world, "Players", name))})
	}
	return world, out
}
