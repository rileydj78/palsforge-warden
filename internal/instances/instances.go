// Package instances lets several Wardens on one machine (one per game server) find each other.
// Each running Warden writes a small file in a shared folder every minute; the panel lists the
// others so an admin can switch between servers. Each Warden stays fully separate (own data,
// ports, task), so one server's trouble never touches another's.
package instances

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Entry is one running Warden.
type Entry struct {
	Name       string    `json:"name"` // "" for the main one
	ServerName string    `json:"serverName"`
	ServerDir  string    `json:"-"`
	Local      string    `json:"local"`  // 127.0.0.1:port (no sign-in, this machine only)
	Admin      string    `json:"admin"`  // 127.0.0.1:port (sign-in)
	Public     string    `json:"public"` // 127.0.0.1:port (status page)
	Remote     string    `json:"remote,omitempty"`
	Tag        string    `json:"tag"` // the backup name prefix
	Updated    time.Time `json:"updated"`
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,19}$`)

// ValidName says whether a name can be an instance name ("main" is the unnamed Warden's).
func ValidName(n string) error {
	if !nameRe.MatchString(n) {
		return errors.New("an instance name is 1-20 lowercase letters, digits or dashes, e.g. pvp or test-2")
	}
	if n == "main" {
		return errors.New(`"main" is the name of the first Warden on this machine; pick another`)
	}
	return nil
}

// Tag is a named instance's default backup prefix: "pal" plus its name (letters and digits).
func Tag(name string) string {
	t := "pal" + strings.ReplaceAll(name, "-", "")
	if len(t) > 20 {
		t = t[:20]
	}
	return t
}

var (
	localRe  = regexp.MustCompile(`^127\.0\.0\.1:[0-9]{2,5}$`)
	remoteRe = regexp.MustCompile(`^https://[a-z0-9-]+(\.[a-z0-9-]+)*\.ts\.net(:[0-9]{2,5})?/$`)
)

// valid drops entries anyone could have planted: the switcher only ever links to this machine's
// own loopback ports or a Tailscale address.
func valid(e Entry) bool {
	if e.Name != "" && ValidName(e.Name) != nil {
		return false
	}
	if !localRe.MatchString(e.Local) || !localRe.MatchString(e.Admin) {
		return false
	}
	return e.Remote == "" || remoteRe.MatchString(e.Remote)
}

// Dir is where the entries live (under Warden's main data folder).
func Dir(mainData string) string { return filepath.Join(mainData, "instances.d") }

func file(mainData, name string) string {
	if name == "" {
		name = "main"
	}
	return filepath.Join(Dir(mainData), name+".json")
}

// Announce writes (or refreshes) this Warden's entry.
func Announce(mainData string, e Entry) error {
	if err := os.MkdirAll(Dir(mainData), 0o755); err != nil {
		return err
	}
	e.Updated = time.Now()
	b, _ := json.MarshalIndent(e, "", "  ")
	tmp := file(mainData, e.Name) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file(mainData, e.Name))
}

// List returns the Wardens seen in the last 3 minutes, main first, then by name.
func List(mainData string) []Entry {
	ms, _ := filepath.Glob(filepath.Join(Dir(mainData), "*.json"))
	var out []Entry
	for _, m := range ms {
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var e Entry
		if json.Unmarshal(b, &e) != nil || time.Since(e.Updated) > 3*time.Minute || !valid(e) {
			continue
		}
		if want := strings.TrimSuffix(filepath.Base(m), ".json"); want != map[bool]string{true: "main", false: e.Name}[e.Name == ""] {
			continue // a file must be named after the instance it describes
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Name == "") != (out[j].Name == "") {
			return out[i].Name == ""
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Ports picks the local, admin and status-page ports for a new instance: the main Warden uses
// 8282-8284; instance n uses 8282+10n onward, skipping any in use by a known instance or by
// anything listening.
func Ports(mainData string) (local, admin, public string) {
	used := map[int]bool{}
	ms, _ := filepath.Glob(filepath.Join(Dir(mainData), "*.json"))
	for _, m := range ms {
		var e Entry
		if b, err := os.ReadFile(m); err == nil && json.Unmarshal(b, &e) == nil {
			for _, a := range []string{e.Local, e.Admin, e.Public} {
				if _, p, err := net.SplitHostPort(a); err == nil {
					n, _ := strconv.Atoi(p)
					used[n] = true
				}
			}
		}
	}
	free := func(p int) bool {
		if used[p] {
			return false
		}
		ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
		if err != nil {
			return false
		}
		ln.Close()
		return true
	}
	for base := 8292; base < 9000; base += 10 {
		if free(base) && free(base+1) && free(base+2) {
			a := func(p int) string { return "127.0.0.1:" + strconv.Itoa(p) }
			return a(base), a(base + 1), a(base + 2)
		}
	}
	return "", "", ""
}
