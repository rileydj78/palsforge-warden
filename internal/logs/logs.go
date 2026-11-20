// Package logs reads the end of the server's log files for the panel, with player IP addresses
// masked (the panel never shows them) and anything that looks like a password removed.
package logs

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Source is one log the panel can show.
type Source struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Path  string `json:"-"`
}

// Sources lists the logs that exist for this server.
func Sources(serverDir, dataDir string) []Source {
	cands := []Source{
		{"warden", "Warden", filepath.Join(dataDir, "warden.log")},
		{"console", "Server console", filepath.Join(dataDir, "server-console.log")},
		{"game", "Palworld (Pal.log)", filepath.Join(serverDir, "Pal", "Saved", "Logs", "Pal.log")},
	}
	if p := newest(filepath.Join(serverDir, "Pal", "Binaries", "Win64", "PalDefender", "Logs")); p != "" {
		cands = append(cands, Source{"paldefender", "PalDefender", p})
	}
	var out []Source
	for _, c := range cands {
		if st, err := os.Stat(c.Path); err == nil && !st.IsDir() {
			out = append(out, c)
		}
	}
	return out
}

// newest returns the most recently written file in dir (PalDefender writes one log per day).
func newest(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	type f struct {
		p string
		t int64
	}
	var fs []f
	for _, e := range ents {
		if info, err := e.Info(); err == nil && !e.IsDir() {
			fs = append(fs, f{filepath.Join(dir, e.Name()), info.ModTime().UnixNano()})
		}
	}
	if len(fs) == 0 {
		return ""
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].t > fs[j].t })
	return fs[0].p
}

var (
	ipv4   = regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\b`)
	secret = regexp.MustCompile(`(?i)(admin ?password|password|token|webhook)(["']?\s*[:=]\s*["']?)[^\s,"')]+`)
)

// Clean masks IP addresses (except loopback and private ranges the admin owns) and secrets.
func Clean(line string) string {
	line = secret.ReplaceAllString(line, "${1}${2}(hidden)")
	return ipv4.ReplaceAllStringFunc(line, func(ip string) string {
		if strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "0.") {
			return ip
		}
		m := ipv4.FindStringSubmatch(ip)
		return m[1] + ".x.x.x"
	})
}

// Tail returns up to n of the last lines of a source, cleaned, optionally only those containing q.
func Tail(src Source, n int, q string) ([]string, error) {
	f, err := os.Open(src.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()
	st, _ := f.Stat()
	window := int64(512 << 10)
	if q != "" {
		window = 8 << 20 // search further back
	}
	off := max(int64(0), st.Size()-window)
	f.Seek(off, io.SeekStart)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	q = strings.ToLower(q)
	var lines []string
	first := off > 0
	for sc.Scan() {
		if first { // the first line may be cut in half
			first = false
			continue
		}
		l := strings.TrimRight(sc.Text(), "\r")
		if q != "" && !strings.Contains(strings.ToLower(l), q) {
			continue
		}
		lines = append(lines, Clean(l))
		if len(lines) > n*2 {
			lines = lines[len(lines)-n:]
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if lines == nil {
		lines = []string{}
	}
	return lines, sc.Err()
}
