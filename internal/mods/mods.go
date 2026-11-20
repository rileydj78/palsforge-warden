// Package mods finds the server's mods, spots common mistakes, and turns mods on and off by
// moving them to (and back from) Warden's own "disabled" area. Nothing is ever deleted.
//
// Where mods live (relative to the server folder):
//
//	Pal/Content/Paks/~mods                         PAK mods (Windows and Linux)
//	Pal/Binaries/Win64/ue4ss/Mods/<mod>            UE4SS Lua mods (Windows only; need UE4SS)
//	Pal/Binaries/Win64/ue4ss/Mods/PalSchema/mods/<mod>  PalSchema data mods (need UE4SS + PalSchema)
//	Pal/Binaries/Win64/PalDefender                 the PalDefender server plugin
package mods

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Kinds.
const (
	Pak       = "pak"
	Lua       = "ue4ss"
	PalSchema = "palschema"
	Plugin    = "plugin"
)

// Mod is one installed (or disabled) mod.
type Mod struct {
	ID      string    `json:"id"` // kind/name: stable while it moves between on and off
	Name    string    `json:"name"`
	Kind    string    `json:"kind"`
	Enabled bool      `json:"enabled"`
	Size    int64     `json:"size"`
	Files   int       `json:"files"`
	Changed time.Time `json:"changed"`
	Pending string    `json:"pending,omitempty"` // "enable" or "disable" waiting for a stop
	path    string
}

// Problem is a likely mistake, explained.
type Problem struct {
	Mod  string `json:"mod,omitempty"`
	Text string `json:"text"`
}

// Manager works on one server folder; disabled mods live under dataDir/mods-disabled.
type Manager struct {
	ServerDir string
	DataDir   string
	mu        sync.Mutex
}

func (m *Manager) live(kind string) string {
	s := m.ServerDir
	switch kind {
	case Pak:
		return filepath.Join(s, "Pal", "Content", "Paks", "~mods")
	case Lua:
		return filepath.Join(s, "Pal", "Binaries", "Win64", "ue4ss", "Mods")
	case PalSchema:
		return filepath.Join(s, "Pal", "Binaries", "Win64", "ue4ss", "Mods", "PalSchema", "mods")
	case Plugin:
		return filepath.Join(s, "Pal", "Binaries", "Win64")
	}
	return ""
}

func (m *Manager) off(kind string) string { return filepath.Join(m.DataDir, "mods-disabled", kind) }

// UE4SS's own folders inside Mods that aren't user mods.
var ue4ssBuiltIn = map[string]bool{"shared": true, "palschema": true, "keybinds": true, "bpml_genericfunctions": true, "bpmodloadermod": true, "consolecommandsmod": true, "consoleenablermod": true, "cheatmanagerenablermod": true, "linetracemod": true, "splitscreenmod": true, "jsbridge": true}

// List finds every mod, on and off, and what's waiting to change.
func (m *Manager) List() ([]Mod, []Problem, error) {
	var out []Mod
	add := func(kind, dir string, enabled bool, pick func(fs.DirEntry) bool) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if !pick(e) {
				continue
			}
			p := filepath.Join(dir, e.Name())
			size, n, t := measure(p)
			out = append(out, Mod{ID: kind + "/" + e.Name(), Name: e.Name(), Kind: kind, Enabled: enabled, Size: size, Files: n, Changed: t, path: p})
		}
	}
	isPak := func(e fs.DirEntry) bool {
		n := strings.ToLower(e.Name())
		return e.IsDir() || strings.HasSuffix(n, ".pak") || strings.HasSuffix(n, ".utoc") || strings.HasSuffix(n, ".ucas")
	}
	isLua := func(e fs.DirEntry) bool { return e.IsDir() && !ue4ssBuiltIn[strings.ToLower(e.Name())] }
	isDir := func(e fs.DirEntry) bool { return e.IsDir() }
	isPlugin := func(e fs.DirEntry) bool { return e.IsDir() && strings.EqualFold(e.Name(), "PalDefender") }
	for _, on := range []bool{true, false} {
		dir := m.live
		if !on {
			dir = m.off
		}
		add(Pak, dir(Pak), on, isPak)
		add(Lua, dir(Lua), on, isLua)
		add(PalSchema, dir(PalSchema), on, isDir)
		add(Plugin, dir(Plugin), on, isPlugin)
	}
	pend := m.pending()
	for i := range out {
		out[i].Pending = pend[out[i].ID]
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, m.problems(out), nil
}

func measure(p string) (size int64, n int, t time.Time) {
	filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				size += info.Size()
				n++
				if info.ModTime().After(t) {
					t = info.ModTime()
				}
			}
		}
		return nil
	})
	return
}

var luaMain = regexp.MustCompile(`(?i)^scripts[\\/]+main\.lua$`)

func (m *Manager) problems(list []Mod) []Problem {
	var out []Problem
	hasUE4SS := exists(filepath.Join(m.ServerDir, "Pal", "Binaries", "Win64", "ue4ss", "UE4SS.dll")) || exists(filepath.Join(m.ServerDir, "Pal", "Binaries", "Win64", "UE4SS.dll"))
	hasPS := exists(filepath.Join(m.live(Lua), "PalSchema"))
	for _, md := range list {
		if !md.Enabled {
			continue
		}
		switch md.Kind {
		case Lua:
			if !hasUE4SS {
				out = append(out, Problem{md.Name, "is a UE4SS Lua mod, but UE4SS isn't installed, so it won't load"})
			}
			if runtime.GOOS != "windows" {
				out = append(out, Problem{md.Name, "is a UE4SS mod; UE4SS only works on the Windows server"})
			}
			found := false
			filepath.WalkDir(md.path, func(p string, d fs.DirEntry, err error) error {
				if rel, _ := filepath.Rel(md.path, p); luaMain.MatchString(rel) {
					found = true
				}
				return nil
			})
			if !found && exists(filepath.Join(md.path, "mod.json")) {
				out = append(out, Problem{md.Name, "looks like a PalSchema mod in the UE4SS Mods folder; it belongs in ue4ss/Mods/PalSchema/mods"})
			} else if !found {
				out = append(out, Problem{md.Name, "has no Scripts/main.lua, so UE4SS won't run it (a folder nested one level too deep?)"})
			}
		case PalSchema:
			if !hasPS {
				out = append(out, Problem{md.Name, "is a PalSchema mod, but PalSchema isn't installed"})
			}
		case Pak:
			if strings.HasSuffix(strings.ToLower(md.Name), ".utoc") || strings.HasSuffix(strings.ToLower(md.Name), ".ucas") {
				base := strings.TrimSuffix(strings.TrimSuffix(md.Name, filepath.Ext(md.Name)), "")
				if !exists(filepath.Join(m.live(Pak), base+".pak")) {
					out = append(out, Problem{md.Name, "has no matching .pak file next to it"})
				}
			}
		}
	}
	return out
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// ---- turning mods on and off

func (m *Manager) pendingPath() string { return filepath.Join(m.DataDir, "mods-pending.json") }

func (m *Manager) pending() map[string]string {
	out := map[string]string{}
	if b, err := os.ReadFile(m.pendingPath()); err == nil {
		json.Unmarshal(b, &out)
	}
	return out
}

func (m *Manager) savePending(p map[string]string) error {
	if len(p) == 0 {
		os.Remove(m.pendingPath())
		return nil
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(m.pendingPath(), b, 0o600)
}

// Queue records a change to make at the next stop (or now, through ApplyPending).
func (m *Manager) Queue(id, op string) error {
	if op != "enable" && op != "disable" {
		return errors.New("enable or disable")
	}
	list, _, _ := m.List()
	var md *Mod
	for i := range list {
		if list[i].ID == id {
			md = &list[i]
		}
	}
	if md == nil {
		return fmt.Errorf("no mod %q", id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pending()
	if (op == "enable") == md.Enabled {
		delete(p, id) // already in that state: cancel any opposite request
	} else {
		p[id] = op
	}
	return m.savePending(p)
}

// ApplyPending makes the waiting changes. Call it only while the server is stopped (the files are
// locked while it runs). It returns what it did, one line per mod.
func (m *Manager) ApplyPending() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pending()
	var done []string
	var errs []error
	for id, op := range p {
		kind, name, ok := strings.Cut(id, "/")
		if !ok || name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
			delete(p, id)
			continue
		}
		from, to := m.live(kind), m.off(kind)
		if op == "enable" {
			from, to = to, from
		}
		if err := os.MkdirAll(to, 0o755); err != nil {
			errs = append(errs, err)
			continue
		}
		src, dst := filepath.Join(from, name), filepath.Join(to, name)
		if exists(dst) {
			errs = append(errs, fmt.Errorf("%s: something with that name is already there", name))
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		done = append(done, op+"d "+name)
		delete(p, id)
	}
	if err := m.savePending(p); err != nil {
		errs = append(errs, err)
	}
	return done, errors.Join(errs...)
}
