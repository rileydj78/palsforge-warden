package backup

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makeServer(t *testing.T) string {
	root := t.TempDir()
	files := map[string]string{
		"Pal/Saved/SaveGames/0/world/Level.sav":                         "world",
		"Pal/Saved/Config/WindowsServer/PalWorldSettings.ini":           "settings",
		"Pal/Binaries/Win64/PalDefender/Config.json":                    "{}",
		"Pal/Saved/SaveGames/0/world/backup/world/2026.11.19/Level.sav": "game's own rolling backups",
	}
	for p, c := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(c), 0o644)
	}
	return root
}

func TestMakeVerifyCopyAndChecksum(t *testing.T) {
	root, out, second := makeServer(t), t.TempDir(), t.TempDir()
	skip := func(rel string) bool { return strings.Contains(rel, "/backup/") }
	a, err := Make(out, FileName("pal", Data, time.Now()), root, []string{"Pal/Saved/SaveGames", "Pal/Saved/Config", "Pal/Binaries/Win64/PalDefender", "Pal/Mods-not-there"}, skip)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Path + ".part"); !os.IsNotExist(err) {
		t.Fatal("the .part file must be gone after success")
	}
	z, err := zip.OpenReader(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range z.File {
		names[f.Name] = true
	}
	z.Close()
	if !names["Pal/Saved/SaveGames/0/world/Level.sav"] || !names["Pal/Binaries/Win64/PalDefender/Config.json"] || len(names) != 3 {
		t.Fatalf("archive contents: %v", names)
	}
	if sum, _ := FileSHA256(a.Path); sum != a.SHA256 {
		t.Fatal("checksum mismatch")
	}
	if err := CopyVerified(a, second); err != nil {
		t.Fatal(err)
	}
	if sum, _ := FileSHA256(filepath.Join(second, a.Name)); sum != a.SHA256 {
		t.Fatal("copy checksum mismatch")
	}
	// A corrupted source must not produce a copy with the final name.
	os.WriteFile(a.Path, []byte("corrupt"), 0o644)
	third := t.TempDir()
	if err := CopyVerified(a, third); err == nil {
		t.Fatal("a copy that doesn't match its checksum must fail")
	}
	if _, err := os.Stat(filepath.Join(third, a.Name)); !os.IsNotExist(err) {
		t.Fatal("a failed copy must not leave the final name behind")
	}
}

func TestNothingToBackUp(t *testing.T) {
	if _, err := Make(t.TempDir(), "x-data-20261119-0400.zip", t.TempDir(), []string{"missing"}, nil); err == nil {
		t.Fatal("an empty backup must be an error, not a silent empty zip")
	}
}

func TestPruneKeepsDailyAndWeekly(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 6, 1, 4, 0, 0, 0, time.Local)
	for i := 0; i < 90; i++ { // 90 nightly data backups
		name := FileName("pal", Data, start.AddDate(0, 0, i))
		os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "pvp-data-20260601-0400.zip"), []byte("x"), 0o644) // another server's
	removed, err := Prune(dir, DefaultPolicy, "pal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pvp-data-20260601-0400.zip")); err != nil {
		t.Fatal("another server's backup must never be pruned")
	}
	left, _ := ListTag(dir, "pal")
	if len(left)+len(removed) != 90 {
		t.Fatal("counts don't add up")
	}
	// 14 newest days, plus weeks back to 8 distinct weeks (some overlap with the 14 days).
	if len(left) < 14 || len(left) > 14+8 {
		t.Fatalf("kept %d", len(left))
	}
	if left[0].When != start.AddDate(0, 0, 89) {
		t.Fatal("the newest must be kept")
	}
}

func TestExtractRefusesEscapes(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.zip")
	f, _ := os.Create(bad)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("../../evil.txt")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	root := t.TempDir()
	if _, err := Extract(bad, root, []string{"Pal/Saved"}); err == nil {
		t.Fatal("a path that climbs out must be refused")
	}
	good := filepath.Join(dir, "good.zip")
	f, _ = os.Create(good)
	zw = zip.NewWriter(f)
	w, _ = zw.Create("Pal/Saved/SaveGames/0/W/Level.sav")
	w.Write([]byte("world"))
	w, _ = zw.Create("Pal/Binaries/Win64/evil.dll")
	w.Write([]byte("x"))
	zw.Close()
	f.Close()
	if _, err := Extract(good, root, []string{"Pal/Saved"}); err == nil {
		t.Fatal("files outside the allowed places must be refused")
	}
	if _, err := os.Stat(filepath.Join(root, "Pal/Saved/SaveGames/0/W/Level.sav")); !os.IsNotExist(err) {
		t.Fatal("nothing may be written when any entry is refused")
	}
}
