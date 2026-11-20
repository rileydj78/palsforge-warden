package mods

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// MaxZip is the largest mod archive Warden accepts.
const MaxZip = 1 << 30

var safeName = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

// Install unpacks a mod archive into Warden's disabled area and queues it to be turned on (at the
// next stop, or at once if the server is stopped and the caller applies). It works out the kind
// from what's inside and refuses archives it can't place, explaining why. Returns the mod's id.
func (m *Manager) Install(zipPath, name string) (string, error) {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", errors.New("that isn't a zip file Warden can read")
	}
	defer z.Close()
	var paks []*zip.File
	luaRoot, schemaRoot := "", ""
	for _, f := range z.File {
		n := path.Clean(strings.ReplaceAll(f.Name, `\`, "/"))
		if strings.HasPrefix(n, "../") || strings.HasPrefix(n, "/") || n == ".." {
			return "", fmt.Errorf("refusing %q: it would land outside the mod's folder", f.Name)
		}
		low := strings.ToLower(n)
		switch {
		case strings.HasSuffix(low, ".pak") || strings.HasSuffix(low, ".utoc") || strings.HasSuffix(low, ".ucas"):
			paks = append(paks, f)
		case strings.HasSuffix(low, "scripts/main.lua"):
			root := path.Dir(path.Dir(n)) // the folder that holds Scripts/
			if luaRoot == "" || len(root) < len(luaRoot) {
				luaRoot = root
			}
		case path.Base(low) == "mod.json":
			if root := path.Dir(n); schemaRoot == "" || len(root) < len(schemaRoot) {
				schemaRoot = root
			}
		}
	}
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	base = strings.TrimSpace(safeName.ReplaceAllString(base, ""))
	switch {
	case luaRoot != "":
		folder := base
		if luaRoot != "." {
			folder = safeName.ReplaceAllString(path.Base(luaRoot), "")
		}
		return m.unpackDir(z, Lua, luaRoot, folder)
	case schemaRoot != "":
		folder := base
		if schemaRoot != "." {
			folder = safeName.ReplaceAllString(path.Base(schemaRoot), "")
		}
		return m.unpackDir(z, PalSchema, schemaRoot, folder)
	case len(paks) > 0:
		dst := m.off(Pak)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return "", err
		}
		var first string
		for _, f := range paks {
			fn := safeName.ReplaceAllString(path.Base(strings.ReplaceAll(f.Name, `\`, "/")), "")
			if fn == "" {
				continue
			}
			if exists(filepath.Join(m.live(Pak), fn)) || exists(filepath.Join(dst, fn)) {
				return "", fmt.Errorf("a mod file named %s is already installed; turn it off and remove it first", fn)
			}
			if err := writeEntry(f, filepath.Join(dst, fn)); err != nil {
				return "", err
			}
			if strings.HasSuffix(strings.ToLower(fn), ".pak") && first == "" {
				first = fn
			} else if first == "" {
				first = fn
			}
			// every file of the mod is queued; the id of the .pak is returned for the message
			if err := m.queueRaw(Pak+"/"+fn, "enable"); err != nil {
				return "", err
			}
		}
		return Pak + "/" + first, nil
	}
	return "", errors.New("Warden can't tell what kind of mod this is: no .pak files, no Scripts/main.lua (UE4SS) and no mod.json (PalSchema). Check the mod's instructions")
}

func (m *Manager) unpackDir(z *zip.ReadCloser, kind, root, folder string) (string, error) {
	if strings.Trim(folder, ". ") == "" {
		return "", errors.New("the mod's folder has no usable name")
	}
	dst := filepath.Join(m.off(kind), folder)
	if exists(dst) || exists(filepath.Join(m.live(kind), folder)) {
		return "", fmt.Errorf("a mod named %s is already installed; turn it off and remove it first", folder)
	}
	prefix := ""
	if root != "." {
		prefix = root + "/"
	}
	n := 0
	for _, f := range z.File {
		name := path.Clean(strings.ReplaceAll(f.Name, `\`, "/"))
		if f.FileInfo().IsDir() || (prefix != "" && !strings.HasPrefix(name, prefix)) {
			continue
		}
		rel := strings.TrimPrefix(name, prefix)
		if err := writeEntry(f, filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
			os.RemoveAll(dst)
			return "", err
		}
		n++
	}
	if n == 0 {
		return "", errors.New("the mod's folder is empty")
	}
	id := kind + "/" + folder
	return id, m.queueRaw(id, "enable")
}

func writeEntry(f *zip.File, dst string) error {
	if f.UncompressedSize64 > MaxZip {
		return fmt.Errorf("%s is too large", f.Name)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(src, MaxZip))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// queueRaw records a pending change without looking the mod up (used right after unpacking).
func (m *Manager) queueRaw(id, op string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pending()
	p[id] = op
	return m.savePending(p)
}
