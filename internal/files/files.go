// Package files is Warden's small file manager: the server's config, mod, PalDefender and log
// folders only. World saves are left to backups and restores. Every edit keeps the previous
// version and a delete moves the file to Warden's bin, so nothing done here is final.
package files

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Root is one folder the file manager may show.
type Root struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Dir  string `json:"-"`
}

// Roots lists the folders that exist for this server.
func Roots(serverDir string) []Root {
	plat := "WindowsServer"
	if runtime.GOOS != "windows" {
		plat = "LinuxServer"
	}
	all := []Root{
		{"config", "Game config (" + plat + ")", filepath.Join(serverDir, "Pal", "Saved", "Config", plat)},
		{"mods", "UE4SS mods", filepath.Join(serverDir, "Pal", "Binaries", "Win64", "ue4ss", "Mods")},
		{"mods-old", "UE4SS mods (older layout)", filepath.Join(serverDir, "Pal", "Binaries", "Win64", "Mods")},
		{"paks", "Pak mods (~mods)", filepath.Join(serverDir, "Pal", "Content", "Paks", "~mods")},
		{"paldefender", "PalDefender", filepath.Join(serverDir, "Pal", "Binaries", "Win64", "PalDefender")},
		{"logs", "Server logs", filepath.Join(serverDir, "Pal", "Saved", "Logs")},
	}
	var out []Root
	for _, r := range all {
		if fi, err := os.Stat(r.Dir); err == nil && fi.IsDir() {
			out = append(out, r)
		}
	}
	return out
}

// Manager works inside the roots, keeping old versions and deleted files under DataDir.
type Manager struct {
	Roots   []Root
	DataDir string // Warden's data folder: file-history/ and file-bin/ live here
}

// Limits.
const (
	MaxEdit   = 1 << 20  // text files up to 1 MB can be edited in the browser
	MaxUpload = 64 << 20 // uploads up to 64 MB
	keepVers  = 20
)

// Never shown: secrets Warden itself manages, and anything that isn't a plain file.
var hidden = []string{"tokens"} // PalDefender's RESTAPI/Tokens (API tokens, Warden's included)

var editable = []string{".ini", ".json", ".txt", ".lua", ".cfg", ".toml", ".yaml", ".yml", ".xml", ".md", ".log", ".csv", ".conf"}

func (m *Manager) root(id string) (Root, error) {
	for _, r := range m.Roots {
		if r.ID == id {
			return r, nil
		}
	}
	return Root{}, errors.New("that folder isn't one Warden manages")
}

// resolve turns (root, relative path) into an absolute path that is certainly inside the root,
// after following any links.
func (m *Manager) resolve(id, rel string) (string, Root, error) {
	r, err := m.root(id)
	if err != nil {
		return "", r, err
	}
	rel = filepath.FromSlash(strings.TrimPrefix(rel, "/"))
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || slices.Contains(strings.Split(rel, string(filepath.Separator)), "..") {
		return "", r, errors.New("that path isn't allowed")
	}
	for _, part := range strings.Split(strings.ToLower(rel), string(filepath.Separator)) {
		if slices.Contains(hidden, part) {
			return "", r, errors.New("that folder holds secrets Warden manages; it isn't shown")
		}
	}
	base, err := filepath.EvalSymlinks(r.Dir)
	if err != nil {
		return "", r, err
	}
	p := filepath.Join(base, rel)
	// Follow links on the deepest part that exists; the rest (a new file or folder) is joined on.
	existing, rest := p, ""
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", r, errors.New("that path doesn't exist")
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", r, err
	}
	real = filepath.Join(real, rest)
	if out, err := filepath.Rel(base, real); err != nil || out == ".." || strings.HasPrefix(out, ".."+string(filepath.Separator)) {
		return "", r, errors.New("that path leads outside the folder")
	}
	return real, r, nil
}

// Entry is one row of a listing.
type Entry struct {
	Name     string `json:"name"`
	Dir      bool   `json:"dir"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Editable bool   `json:"editable"`
}

// List shows one folder.
func (m *Manager) List(id, rel string) ([]Entry, error) {
	p, _, err := m.resolve(id, rel)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := []Entry{}
	for _, d := range des {
		if slices.Contains(hidden, strings.ToLower(d.Name())) {
			continue
		}
		fi, err := d.Info()
		if err != nil || (!fi.IsDir() && !fi.Mode().IsRegular()) {
			continue
		}
		e := Entry{Name: d.Name(), Dir: fi.IsDir(), Modified: fi.ModTime().Unix()}
		if !e.Dir {
			e.Size = fi.Size()
			e.Editable = fi.Size() <= MaxEdit && slices.Contains(editable, strings.ToLower(filepath.Ext(d.Name())))
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// Open opens a file for download.
func (m *Manager) Open(id, rel string) (*os.File, os.FileInfo, error) {
	p, _, err := m.resolve(id, rel)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, nil, err
	}
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		f.Close()
		return nil, nil, errors.New("that isn't a file")
	}
	return f, fi, nil
}

// ReadText returns an editable file's text and its modified time (sent back on save, so a file
// changed meanwhile isn't overwritten).
func (m *Manager) ReadText(id, rel string) (string, int64, error) {
	p, _, err := m.resolve(id, rel)
	if err != nil {
		return "", 0, err
	}
	if !slices.Contains(editable, strings.ToLower(filepath.Ext(p))) {
		return "", 0, errors.New("only text files (.ini, .json, .lua, .txt …) open here; download others")
	}
	fi, err := os.Stat(p)
	if err != nil {
		return "", 0, err
	}
	if fi.Size() > MaxEdit {
		return "", 0, errors.New("that file is over 1 MB; download it instead")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", 0, err
	}
	if !utf8.Valid(b) {
		return "", 0, errors.New("that file isn't plain text; download it instead")
	}
	return string(b), fi.ModTime().UnixNano(), nil
}

// WriteText saves an edited text file. The previous version is kept (the last 20 per file).
// modified must be what ReadText returned (0 for a new file).
func (m *Manager) WriteText(id, rel, text string, modified int64) error {
	p, r, err := m.resolve(id, rel)
	if err != nil {
		return err
	}
	if !slices.Contains(editable, strings.ToLower(filepath.Ext(p))) {
		return errors.New("only text files can be edited here")
	}
	if len(text) > MaxEdit || !utf8.ValidString(text) {
		return errors.New("the text is too long or not plain text")
	}
	if fi, err := os.Stat(p); err == nil {
		if fi.ModTime().UnixNano() != modified {
			return errors.New("the file changed since you opened it (the server or a mod may have written it); reopen it and make your change again")
		}
		if err := m.keepVersion(r, rel, p); err != nil {
			return fmt.Errorf("couldn't keep the old version, so nothing was changed: %w", err)
		}
	} else if modified != 0 {
		return errors.New("the file was removed since you opened it")
	}
	tmp := p + ".warden-new"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Upload stores a file (a mod's files, a config). An existing file is kept as a version first.
func (m *Manager) Upload(id, rel string, body io.Reader) error {
	p, r, err := m.resolve(id, rel)
	if err != nil {
		return err
	}
	name := filepath.Base(p)
	if name == "." || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `<>:"|?*`) {
		return errors.New("that file name isn't allowed")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if _, _, err := m.resolve(id, rel); err != nil { // the folder may be new: check again
		return err
	}
	if fi, err := os.Stat(p); err == nil {
		if fi.IsDir() {
			return errors.New("a folder already has that name")
		}
		if err := m.keepVersion(r, rel, p); err != nil {
			return err
		}
	}
	tmp := p + ".warden-new"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(body, MaxUpload+1))
	f.Close()
	if err == nil && n > MaxUpload {
		err = errors.New("uploads are limited to 64 MB")
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, p)
}

// Trash moves a file or folder to Warden's bin (data/file-bin/<time>/<root>/<path>).
func (m *Manager) Trash(id, rel string) (string, error) {
	p, r, err := m.resolve(id, rel)
	if err != nil {
		return "", err
	}
	if strings.Trim(filepath.ToSlash(rel), "/") == "" {
		return "", errors.New("the folder itself can't be deleted")
	}
	dst := filepath.Join(m.DataDir, "file-bin", time.Now().Format("20060102-150405"), r.ID, filepath.FromSlash(strings.TrimPrefix(rel, "/")))
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", err
	}
	if err := os.Rename(p, dst); err != nil {
		return "", fmt.Errorf("couldn't move it to Warden's bin (is the server using it?): %w", err)
	}
	return dst, nil
}

// Versions lists the kept versions of one file, newest first.
func (m *Manager) Versions(id, rel string) ([]string, error) {
	if _, _, err := m.resolve(id, rel); err != nil {
		return nil, err
	}
	des, err := os.ReadDir(m.histDir(id, rel))
	if err != nil {
		return []string{}, nil
	}
	var out []string
	for _, d := range des {
		out = append(out, d.Name())
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
}

// ReadVersion returns one kept version's text.
func (m *Manager) ReadVersion(id, rel, version string) (string, error) {
	if _, _, err := m.resolve(id, rel); err != nil {
		return "", err
	}
	if version == "" || strings.ContainsAny(version, `/\`) || strings.Contains(version, "..") {
		return "", errors.New("which version?")
	}
	b, err := os.ReadFile(filepath.Join(m.histDir(id, rel), version))
	if err != nil {
		return "", err
	}
	if len(b) > MaxEdit || !utf8.Valid(b) {
		return "", errors.New("that version isn't plain text")
	}
	return string(b), nil
}

func (m *Manager) histDir(id, rel string) string {
	clean := strings.Trim(filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))), "/")
	return filepath.Join(m.DataDir, "file-history", id, filepath.FromSlash(clean))
}

func (m *Manager) keepVersion(r Root, rel, p string) error {
	dir := m.histDir(r.ID, rel)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	src, err := os.Open(p)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(dir, time.Now().Format("20060102-150405.000")))
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	des, _ := os.ReadDir(dir)
	for i := 0; i < len(des)-keepVers; i++ { // oldest first (names sort by time)
		os.Remove(filepath.Join(dir, des[i].Name()))
	}
	return nil
}
