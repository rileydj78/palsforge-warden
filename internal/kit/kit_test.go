package kit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/config"
)

type fakeGive struct {
	items, pals, msgs int
	fail              bool
}

func (f *fakeGive) GiveItems(ctx context.Context, u, id string, n int) error {
	if f.fail {
		return errors.New("player still loading")
	}
	f.items++
	return nil
}
func (f *fakeGive) GivePal(ctx context.Context, u, id string, lv int) error { f.pals++; return nil }
func (f *fakeGive) Message(ctx context.Context, u, m string) error          { f.msgs++; return nil }

type fakeRec struct{ given map[string]bool }

func (r *fakeRec) KitGiven(u string) bool           { return r.given[u] }
func (r *fakeRec) MarkKit(u string)                 { r.given[u] = true }
func (r *fakeRec) Audit(a, b, c, d string, ok bool) {}

func TestKitOncePerPlayerAfterLoading(t *testing.T) {
	g, rec := &fakeGive{fail: true}, &fakeRec{given: map[string]bool{}}
	k := &Kit{Cfg: config.StarterKit{On: true, Items: "PalSphere:10, Money:5000", Pals: "SheepBall:5", Message: "Welcome {name}"}, Give: g, Rec: rec}
	k.Seen(context.Background(), "steam_1", "Moss")
	if g.items+g.pals != 0 {
		t.Fatal("nothing in the first 30 seconds (still loading in)")
	}
	k.first["steam_1"] = time.Now().Add(-time.Minute)
	k.Seen(context.Background(), "steam_1", "Moss")
	if rec.given["steam_1"] {
		t.Fatal("a kit that didn't arrive at all is retried, not marked given")
	}
	g.fail = false
	k.Seen(context.Background(), "steam_1", "Moss")
	k.Seen(context.Background(), "steam_1", "Moss")
	if g.items != 2 || g.pals != 1 || g.msgs != 1 || !rec.given["steam_1"] {
		t.Fatalf("once, whole kit: %+v", g)
	}
}

func TestKitFormatIsChecked(t *testing.T) {
	if _, err := config.ParseKit("PalSphere:10, Money"); err == nil {
		t.Fatal("an entry without a number must be refused")
	}
	if _, err := config.ParseKit("Bad Name:1"); err == nil {
		t.Fatal("IDs are letters, digits and underscores")
	}
}
