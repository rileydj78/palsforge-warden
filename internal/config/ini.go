// Package config reads what Warden needs from a Palworld server's own files.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Settings holds the values Warden needs from PalWorldSettings.ini.
type Settings struct {
	Path          string            // the file they came from
	RESTEnabled   bool              // RESTAPIEnabled
	RESTPort      int               // RESTAPIPort (default 8212)
	AdminPassword string            // AdminPassword; never sent to a browser
	ServerName    string            // ServerName
	PublicPort    int               // PublicPort
	All           map[string]string // every option, raw, for the settings view
	PasswordIsSet bool
}

// The file holds one line: OptionSettings=(Key=Value,Key="quoted, value",...)
var optionRe = regexp.MustCompile(`(?s)OptionSettings=\((.*)\)`)

// SettingsPath returns the usual PalWorldSettings.ini location inside a server folder.
func SettingsPath(serverDir string) string {
	sub := "LinuxServer"
	if runtime.GOOS == "windows" {
		sub = "WindowsServer"
	}
	return filepath.Join(serverDir, "Pal", "Saved", "Config", sub, "PalWorldSettings.ini")
}

// ReadSettings parses PalWorldSettings.ini.
func ReadSettings(path string) (*Settings, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := optionRe.FindSubmatch(b)
	if m == nil {
		return nil, errors.New("no OptionSettings line in " + path)
	}
	all := parseOptions(string(m[1]))
	s := &Settings{Path: path, All: all, RESTPort: 8212}
	s.RESTEnabled = strings.EqualFold(all["RESTAPIEnabled"], "True")
	if p, err := strconv.Atoi(all["RESTAPIPort"]); err == nil && p > 0 && p < 65536 {
		s.RESTPort = p
	}
	if p, err := strconv.Atoi(all["PublicPort"]); err == nil {
		s.PublicPort = p
	}
	s.AdminPassword = all["AdminPassword"]
	s.PasswordIsSet = s.AdminPassword != ""
	s.ServerName = all["ServerName"]
	return s, nil
}

// parseOptions splits Key=Value pairs on commas that are not inside quotes or brackets.
func parseOptions(body string) map[string]string {
	out := map[string]string{}
	var cur strings.Builder
	depth, quoted := 0, false
	flush := func() {
		kv := strings.TrimSpace(cur.String())
		cur.Reset()
		if kv == "" {
			return
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		out[strings.TrimSpace(k)] = v
	}
	for _, r := range body {
		switch {
		case r == '"':
			quoted = !quoted
		case !quoted && r == '(':
			depth++
		case !quoted && r == ')':
			depth--
		case !quoted && depth == 0 && r == ',':
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return out
}

// Describe is a short, safe summary for logs (never the password).
func (s *Settings) Describe() string {
	return fmt.Sprintf("%s: REST API %v on port %d, admin password set: %v", s.Path, s.RESTEnabled, s.RESTPort, s.PasswordIsSet)
}
