package backup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/webdav"
)

func TestWebDAVRoundTrip(t *testing.T) {
	dav := &webdav.Handler{FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "me" || p != "app-pass" {
			w.WriteHeader(401)
			return
		}
		dav.ServeHTTP(w, r)
	}))
	defer srv.Close()
	tg := WebDAVTarget{URL: srv.URL + "/palworld backups/", User: "me", Password: "app-pass"}
	dir := t.TempDir()
	keep := filepath.Join(dir, "pal-data-20261120-120000.zip")
	old := filepath.Join(dir, "pal-data-20200101-000000.zip")
	os.WriteFile(keep, []byte("keep me"), 0o644)
	os.WriteFile(old, []byte("old"), 0o644)
	ctx := context.Background()
	for _, p := range []string{keep, old} {
		if err := tg.SendVerified(ctx, Archive{Name: filepath.Base(p), Path: p}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := tg.Mirror(ctx, "pal", func([]string) map[string]bool { return map[string]bool{filepath.Base(keep): true} }); err != nil || n != 1 {
		t.Fatalf("retention: %d %v", n, err)
	}
	names, _ := tg.List(ctx)
	if len(names) != 1 || names[0] != filepath.Base(keep) {
		t.Fatalf("left: %v (no .part files either)", names)
	}
	tg.Password = "wrong"
	if tg.SendVerified(ctx, Archive{Name: filepath.Base(keep), Path: keep}) == nil {
		t.Fatal("a wrong password must be refused")
	}
}
