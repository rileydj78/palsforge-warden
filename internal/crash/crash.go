// Package crash looks for the likely cause after the game server crashes, from the files the game
// and its mod loader leave behind: Unreal's crash report (Pal/Saved/Crashes/<id>/
// CrashContext.runtime-xml: the error and the call stack), the end of Pal.log, and UE4SS's log.
// It's a hint, not a diagnosis: it names what the files point at, in plain words.
package crash

import (
	"bufio"
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Hint is what the files say.
type Hint struct {
	Summary string   `json:"summary"` // one plain sentence, "" if nothing useful was found
	Details []string `json:"details"` // the lines it's based on
}

// Modules that are always on a game server's call stack, so they don't point at anything.
var ordinary = map[string]bool{
	"palserver-win64-shipping": true, "palserver-win64-shipping-cmd": true, "palserver-linux-shipping": true,
	"kernelbase": true, "kernel32": true, "ntdll": true, "ucrtbase": true, "vcruntime140": true, "msvcp140": true,
	"user32": true, "unknown": true, "libc.so.6": true, "libpthread.so.0": true, "steamclient64": true, "tbb": true, "tbb12": true,
}

var frameRe = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_.\-]+)!`)

type context struct {
	Error string `xml:"RuntimeProperties>ErrorMessage"`
	Type  string `xml:"RuntimeProperties>CrashType"`
	Stack string `xml:"RuntimeProperties>CallStack"`
}

// Find looks at what the game left after a crash that happened after since.
func Find(serverDir, platformLogs string, since time.Time) Hint {
	var h Hint
	var culprit, errMsg string
	gameLog := filepath.Join(serverDir, "Pal", "Saved", "Logs", "Pal.log") // overwritten when the game restarts
	// 1. Unreal's crash report, if one was written for this crash (it keeps a copy of the log).
	if dir := newestDir(filepath.Join(serverDir, "Pal", "Saved", "Crashes"), since); dir != "" {
		if logs, _ := filepath.Glob(filepath.Join(dir, "*.log")); len(logs) > 0 {
			gameLog = logs[0]
		}
		if b, err := os.ReadFile(filepath.Join(dir, "CrashContext.runtime-xml")); err == nil {
			var c context
			if xml.Unmarshal(b, &c) == nil {
				errMsg = firstLine(c.Error)
				if errMsg != "" {
					h.Details = append(h.Details, "crash report: "+errMsg)
				}
				for _, m := range frameRe.FindAllStringSubmatch(c.Stack, 12) {
					name := strings.TrimSuffix(strings.ToLower(m[1]), ".dll")
					if !ordinary[name] && !strings.HasPrefix(name, "api-ms-") {
						culprit = m[1]
						h.Details = append(h.Details, "first non-game code on the crash's call stack: "+m[1])
						break
					}
				}
			}
		}
	}
	// 2. The end of the game's own log.
	for _, l := range tailMatches(gameLog, 400, regexp.MustCompile(`(?i)fatal error|assertion failed|LogWindows: Error|out of memory|Unhandled Exception`), 3) {
		h.Details = append(h.Details, "game log: "+l)
		if errMsg == "" {
			errMsg = l
		}
	}
	// 3. UE4SS (the Lua/C++ mod loader): errors naming a mod.
	modErr := regexp.MustCompile(`(?i)error`)
	modName := regexp.MustCompile(`\[(?:Lua|CPP)?\]?\s*\[([A-Za-z0-9_ .\-]+)\]`)
	for _, l := range tailMatches(filepath.Join(serverDir, platformLogs, "ue4ss", "UE4SS.log"), 300, modErr, 3) {
		h.Details = append(h.Details, "UE4SS: "+l)
		if culprit == "" {
			if m := modName.FindStringSubmatch(l); m != nil {
				culprit = "the UE4SS mod " + m[1]
			}
		}
	}
	switch {
	case strings.Contains(strings.ToLower(errMsg), "out of memory") || strings.Contains(strings.ToLower(errMsg), "ran out of"):
		h.Summary = "It ran out of memory. Palworld's memory grows over days: a daily restart or Warden's memory limit (Settings) keeps it in check."
	case culprit != "" && errMsg != "":
		h.Summary = "Likely cause: " + culprit + " (" + clip(errMsg, 120) + "). If it's a mod, turn it off on the Mods page and see whether the crashes stop."
	case culprit != "":
		h.Summary = "Likely cause: " + culprit + ". If it's a mod, turn it off on the Mods page and see whether the crashes stop."
	case errMsg != "":
		h.Summary = "The game reported: " + clip(errMsg, 160)
	}
	return h
}

func newestDir(root string, since time.Time) string {
	ents, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	type d struct {
		p string
		t time.Time
	}
	var ds []d
	for _, e := range ents {
		if i, err := e.Info(); err == nil && e.IsDir() && i.ModTime().After(since) {
			ds = append(ds, d{filepath.Join(root, e.Name()), i.ModTime()})
		}
	}
	if len(ds) == 0 {
		return ""
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].t.After(ds[j].t) })
	return ds[0].p
}

// tailMatches returns up to max lines matching re among the last n lines of a file.
func tailMatches(path string, n int, re *regexp.Regexp, max int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > 2<<20 {
		f.Seek(st.Size()-2<<20, 0) // the end is what matters
	}
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < max; i-- {
		if re.MatchString(lines[i]) {
			out = append(out, clip(strings.TrimSpace(lines[i]), 200))
		}
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
