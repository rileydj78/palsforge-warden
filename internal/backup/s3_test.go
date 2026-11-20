package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Runs against a real S3-compatible server when WARDEN_S3_TEST is set, e.g.
// WARDEN_S3_TEST="http://127.0.0.1:9000 us-east-1 bucket accesskey secretkey".
func TestS3AgainstARealServer(t *testing.T) {
	spec := os.Getenv("WARDEN_S3_TEST")
	if spec == "" {
		t.Skip("set WARDEN_S3_TEST to run against a real S3-compatible server")
	}
	var tg S3Target
	f := splitFields(spec)
	if len(f) != 5 {
		t.Fatal("WARDEN_S3_TEST needs 5 fields")
	}
	tg = S3Target{Endpoint: f[0], Region: f[1], Bucket: f[2], AccessKey: f[3], SecretKey: f[4], Prefix: "warden test/"}
	dir := t.TempDir()
	keep := filepath.Join(dir, "pal-data-20261120-101500.zip")
	old := filepath.Join(dir, "pal-data-20200101-000000.zip")
	os.WriteFile(keep, []byte("a backup with spaces in its folder"), 0o644)
	os.WriteFile(old, []byte("an old one"), 0o644)
	ctx := context.Background()
	for _, p := range []string{keep, old} {
		if err := tg.SendVerified(ctx, Archive{Name: filepath.Base(p), Path: p}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := tg.Mirror(ctx, "pal", func([]string) map[string]bool { return map[string]bool{filepath.Base(keep): true} })
	if err != nil || n != 1 {
		t.Fatalf("retention should remove exactly the old one: %d %v", n, err)
	}
	names, _ := tg.List(ctx)
	if len(names) != 1 || names[0] != filepath.Base(keep) {
		t.Fatalf("left: %v", names)
	}
	tg.SecretKey = "wrong"
	if tg.SendVerified(ctx, Archive{Name: filepath.Base(keep), Path: keep}) == nil {
		t.Fatal("a wrong key must be refused")
	}
}

func splitFields(s string) []string { return strings.Fields(s) }
