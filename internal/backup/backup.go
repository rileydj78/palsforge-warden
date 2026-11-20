// Package backup makes backups that can be trusted:
//
//   - two kinds per run: "data" (saves, settings, mods, logs: small, the part that matters) and
//     "server" (the whole server folder: an image to rebuild from);
//   - written as name.zip.part and renamed only when complete, so nothing half-written is ever
//     taken for a backup;
//   - a SHA-256 checksum for every archive, written next to it as name.zip.sha256;
//   - copied to a second place and checked against that checksum before a run counts as safe;
//   - retention: the newest 14 of each kind plus one per week for 8 weeks.
package backup

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Kinds of backup.
const (
	Data   = "data"
	Server = "server"
)

// Archive is one finished backup file.
type Archive struct {
	Name   string    `json:"name"`
	Kind   string    `json:"kind"`
	Tag    string    `json:"tag"`
	Pinned string    `json:"pinned,omitempty"` // why it's kept whatever the retention says
	Path   string    `json:"-"`
	Size   int64     `json:"size"`
	SHA256 string    `json:"sha256"`
	When   time.Time `json:"when"`
}

var nameRe = regexp.MustCompile(`^([a-z0-9]+)-(data|server)-(\d{8})-(\d{6}|\d{4})\.zip$`)

// FileName is the archive name for a server tag, kind and time.
func FileName(tag, kind string, t time.Time) string {
	return fmt.Sprintf("%s-%s-%s.zip", tag, kind, t.Format("20060102-150405"))
}

// Make zips the given paths (files or folders, relative to root) into dir/name. It writes
// name.part, fsyncs, computes the checksum while writing, then renames and writes name.sha256.
// skip reports paths (relative to root, slash-separated) to leave out.
func Make(dir, name, root string, paths []string, skip func(string) bool) (*Archive, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	final := filepath.Join(dir, name)
	if _, err := os.Stat(final); err == nil {
		return nil, fmt.Errorf("a backup named %s already exists; nothing was overwritten", name)
	}
	part := final + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("another backup is being written right now: %w", err)
	}
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	zw := zip.NewWriter(io.MultiWriter(f, h))
	added := 0
	for _, p := range paths {
		base := filepath.Join(root, p)
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil // a missing optional folder (no mods yet, say) is fine
				}
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if skip != nil && skip(rel) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			hdr, _ := zip.FileInfoHeader(info)
			hdr.Name, hdr.Method = rel, zip.Deflate
			w, err := zw.CreateHeader(hdr)
			if err != nil {
				return err
			}
			src, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("could not read %s: %w", rel, err)
			}
			_, err = io.Copy(w, src)
			src.Close()
			added++
			return err
		})
		if err != nil {
			zw.Close()
			f.Close()
			os.Remove(part)
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		os.Remove(part)
		return nil, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(part)
		return nil, err
	}
	st, _ := f.Stat()
	f.Close()
	if added == 0 {
		os.Remove(part)
		return nil, errors.New("there was nothing to back up (check the server folder)")
	}
	if err := os.Rename(part, final); err != nil {
		return nil, err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if err := os.WriteFile(final+".sha256", []byte(sum+"  "+name+"\n"), 0o640); err != nil {
		return nil, err
	}
	kind := Data
	if m := nameRe.FindStringSubmatch(name); m != nil {
		kind = m[2]
	}
	return &Archive{Name: name, Kind: kind, Path: final, Size: st.Size(), SHA256: sum, When: time.Now()}, nil
}

// FileSHA256 hashes a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CopyVerified copies an archive (and its .sha256) into dstDir, writing .part first, then checks
// the copy's checksum before renaming it. Only a verified copy ever has the final name.
func CopyVerified(a *Archive, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return fmt.Errorf("the backup place %s isn't reachable: %w", dstDir, err)
	}
	dst := filepath.Join(dstDir, a.Name)
	part := dst + ".part"
	src, err := os.Open(a.Path)
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(part)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		os.Remove(part)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(part)
		return err
	}
	out.Close()
	got, err := FileSHA256(part)
	if err != nil || got != a.SHA256 {
		os.Remove(part)
		return fmt.Errorf("the copy of %s didn't match its checksum; it was not kept", a.Name)
	}
	if err := os.Rename(part, dst); err != nil {
		return err
	}
	return os.WriteFile(dst+".sha256", []byte(a.SHA256+"  "+a.Name+"\n"), 0o640)
}

// List returns the finished archives in dir, newest first.
func List(dir string) ([]Archive, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Archive
	for _, e := range ents {
		m := nameRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		layout := "20060102-150405"
		if len(m[4]) == 4 { // older names: to the minute
			layout = "20060102-1504"
		}
		t, err := time.ParseInLocation(layout, m[3]+"-"+m[4], time.Local)
		if err != nil {
			continue
		}
		info, _ := e.Info()
		a := Archive{Name: e.Name(), Kind: m[2], Tag: m[1], Path: filepath.Join(dir, e.Name()), When: t}
		if info != nil {
			a.Size = info.Size()
		}
		if b, err := os.ReadFile(a.Path + ".keep"); err == nil {
			a.Pinned = strings.TrimSpace(string(b))
			if a.Pinned == "" {
				a.Pinned = "kept"
			}
		}
		if b, err := os.ReadFile(a.Path + ".sha256"); err == nil {
			a.SHA256 = strings.Fields(string(b) + " ")[0]
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].When.After(out[j].When) })
	return out, nil
}

// Policy is how many backups to keep.
type Policy struct {
	Daily  int // newest N of each kind
	Weekly int // plus the newest of each of the last N ISO weeks
}

// DefaultPolicy keeps 14 daily and 8 weekly.
var DefaultPolicy = Policy{Daily: 14, Weekly: 8}

// ListTag is List for one server's backups (a shared folder may hold several servers').
func ListTag(dir, tag string) ([]Archive, error) {
	all, err := List(dir)
	var out []Archive
	for _, a := range all {
		if a.Tag == tag {
			out = append(out, a)
		}
	}
	return out, err
}

// Prune removes archives with this tag in dir that the policy doesn't keep; other servers'
// backups in the same folder are never touched. Returns the names removed.
func Prune(dir string, p Policy, tag string) ([]string, error) {
	all, err := ListTag(dir, tag)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	byKind := map[string][]Archive{}
	for _, a := range all {
		byKind[a.Kind] = append(byKind[a.Kind], a)
	}
	for _, list := range byKind { // each list is newest first
		for i, a := range list {
			if i < p.Daily {
				keep[a.Name] = true
			}
		}
		weeks := map[string]bool{}
		for _, a := range list {
			y, w := a.When.ISOWeek()
			k := fmt.Sprintf("%d-%d", y, w)
			if len(weeks) >= p.Weekly && !weeks[k] {
				break
			}
			if !weeks[k] {
				weeks[k] = true
				keep[a.Name] = true
			}
		}
	}
	var removed []string
	for _, a := range all {
		if !keep[a.Name] && a.Pinned == "" {
			os.Remove(a.Path)
			os.Remove(a.Path + ".sha256")
			os.Remove(a.Path + ".keep")
			removed = append(removed, a.Name)
		}
	}
	return removed, nil
}

// Entry is one file inside an archive.
type Entry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Contents lists an archive's files (for a preview before restoring).
func Contents(path string) ([]Entry, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	out := make([]Entry, 0, len(z.File))
	for _, f := range z.File {
		out = append(out, Entry{Name: f.Name, Size: int64(f.UncompressedSize64)})
	}
	return out, nil
}

// Verify checks a finished archive against its .sha256 file.
func Verify(a Archive) error {
	if a.SHA256 == "" {
		return fmt.Errorf("%s has no checksum file, so it can't be trusted for a restore", a.Name)
	}
	got, err := FileSHA256(a.Path)
	if err != nil {
		return err
	}
	if got != a.SHA256 {
		return fmt.Errorf("%s doesn't match its checksum: the file is damaged", a.Name)
	}
	return nil
}

// Extract unpacks an archive into root. Every entry must stay inside root and under one of the
// allowed prefixes (slash-separated); anything else is refused before a single file is written.
func Extract(path, root string, allowed []string) (int, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return 0, err
	}
	defer z.Close()
	rootAbs, _ := filepath.Abs(root)
	for _, f := range z.File {
		name := filepath.ToSlash(filepath.Clean(filepath.FromSlash(f.Name)))
		ok := false
		for _, a := range allowed {
			if name == a || strings.HasPrefix(name, a+"/") {
				ok = true
			}
		}
		dst, _ := filepath.Abs(filepath.Join(root, filepath.FromSlash(name)))
		if !ok || strings.HasPrefix(name, "../") || filepath.IsAbs(f.Name) || !strings.HasPrefix(dst, rootAbs+string(filepath.Separator)) {
			return 0, fmt.Errorf("refusing to restore %q: it's outside the places a backup may write", f.Name)
		}
	}
	n := 0
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dst := filepath.Join(root, filepath.FromSlash(filepath.Clean(filepath.FromSlash(f.Name))))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return n, err
		}
		src, err := f.Open()
		if err != nil {
			return n, err
		}
		out, err := os.Create(dst)
		if err != nil {
			src.Close()
			return n, err
		}
		_, err = io.Copy(out, src)
		src.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Proof is the result of reading a backup back completely.
type Proof struct {
	Files int    `json:"files"`
	World string `json:"world"` // the world file found, e.g. Pal/Saved/SaveGames/0/<id>/Level.sav
	Bytes int64  `json:"bytes"`
}

// Prove reads every file in an archive (the zip reader checks each file's CRC as it's read) and
// checks that a non-empty world file is inside: a backup that passes can really be restored.
func Prove(path string) (*Proof, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("can't open the backup: %w", err)
	}
	defer z.Close()
	p := &Proof{}
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%s can't be read: %w", f.Name, err)
		}
		n, err := io.Copy(io.Discard, rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s is damaged: %w", f.Name, err)
		}
		p.Files++
		p.Bytes += n
		if strings.HasSuffix(f.Name, "/Level.sav") && n > 0 && !strings.Contains(f.Name, "/backup/") {
			p.World = f.Name
		}
	}
	if p.World == "" {
		return p, errors.New("there's no world file (Level.sav) in it")
	}
	return p, nil
}

// Pin marks a backup to be kept whatever the retention says (e.g. the last one before a crash).
func Pin(dir, name, why string) error {
	if !nameRe.MatchString(name) {
		return errors.New("not a backup name")
	}
	return os.WriteFile(filepath.Join(dir, name+".keep"), []byte(why+"\n"), 0o640)
}
