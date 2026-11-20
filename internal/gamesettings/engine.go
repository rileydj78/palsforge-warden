package gamesettings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Engine.ini holds Unreal Engine tuning (network tick rate, bandwidth). Edits wait in
// engine-pending.ini and are written while the server is stopped, like the game's settings; the
// old file is kept as Engine.ini.warden-<time>.bak.

// EnginePath is the server's Engine.ini next to PalWorldSettings.ini.
func EnginePath(settingsPath string) string {
	return filepath.Join(filepath.Dir(settingsPath), "Engine.ini")
}

func enginePending(dataDir string) string { return filepath.Join(dataDir, "engine-pending.ini") }

// CheckEngine refuses text that isn't an ini file.
func CheckEngine(text string) error {
	if len(text) > 64<<10 {
		return errors.New("Engine.ini is limited to 64 KB here")
	}
	for i, line := range strings.Split(text, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, ";") || strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") || strings.Contains(l, "=") {
			continue
		}
		return errors.New("line " + itoa(i+1) + " isn't a [Section], a Key=Value or a ; comment")
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// QueueEngine stores new Engine.ini text for the next stop.
func QueueEngine(dataDir, text string) error {
	if err := CheckEngine(text); err != nil {
		return err
	}
	return os.WriteFile(enginePending(dataDir), []byte(text), 0o600)
}

// PendingEngine returns waiting Engine.ini text, if any.
func PendingEngine(dataDir string) (string, bool) {
	b, err := os.ReadFile(enginePending(dataDir))
	return string(b), err == nil
}

// ApplyEngine writes waiting Engine.ini text (call only while the server is stopped).
func ApplyEngine(settingsPath, dataDir string) (string, error) {
	text, ok := PendingEngine(dataDir)
	if !ok {
		return "", nil
	}
	p := EnginePath(settingsPath)
	if old, err := os.ReadFile(p); err == nil {
		os.WriteFile(p+".warden-"+time.Now().Format("20060102-150405")+".bak", old, 0o600)
	}
	tmp := p + ".warden-tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		return "", err
	}
	os.Remove(enginePending(dataDir))
	return "Engine.ini written", nil
}
