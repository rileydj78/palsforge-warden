package news

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPatchTitles(t *testing.T) {
	for title, want := range map[string]bool{
		"v1.0.5: Bug fixes": true, "v1.0.2.101103:Bug fixes": true, "Hotfix for crashes": true,
		"Global Popularity Poll 2026 Results & New Update":       true,
		"Palworld × Graveyard Keeper 2 Steam Bundle is Out Now!": false, "Palworld Global Popularity Poll 2026 is LIVE!": false,
	} {
		if IsPatch(title) != want {
			t.Errorf("%q: want %v", title, want)
		}
	}
}

func TestFreshOnlyOnce(t *testing.T) {
	items := `{"appnews":{"newsitems":[{"gid":"1","title":"v1.0.5: Bug fixes","url":"https://x/1","date":1790000000}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, items) }))
	defer srv.Close()
	f := &Feed{DataDir: t.TempDir(), URL: srv.URL}
	if fresh, _ := f.Fetch(context.Background()); len(fresh) != 0 {
		t.Fatal("the first look only learns what's there")
	}
	items = `{"appnews":{"newsitems":[{"gid":"2","title":"v1.0.6: Bug fixes","url":"javascript:alert(1)","date":1790100000},{"gid":"1","title":"v1.0.5: Bug fixes","url":"https://x/1","date":1790000000}]}}`
	fresh, _ := f.Fetch(context.Background())
	if len(fresh) != 1 || fresh[0].ID != "2" || !fresh[0].Patch || fresh[0].URL != "" {
		t.Fatalf("one new patch, unsafe link dropped: %+v", fresh)
	}
	if fresh, _ := f.Fetch(context.Background()); len(fresh) != 0 {
		t.Fatal("never twice")
	}
}
