package broadcast

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/palworld"
)

type fake struct {
	mu        sync.Mutex
	players   []palworld.Player
	announced []string
	private   []string
}

func (f *fake) Players(context.Context) ([]palworld.Player, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]palworld.Player(nil), f.players...), nil
}
func (f *fake) Announce(_ context.Context, m string) error {
	f.mu.Lock()
	f.announced = append(f.announced, m)
	f.mu.Unlock()
	return nil
}

func TestCleanAndWelcomeOnlyNewPlayers(t *testing.T) {
	if got := clean("  join  the\n discord  "); got != "join the discord" {
		t.Fatal(got)
	}
	f := &fake{players: []palworld.Player{{Name: "Already", UserID: "steam_1"}}}
	priv := func(_ context.Context, id, m string) error {
		f.mu.Lock()
		f.private = append(f.private, id+":"+m)
		f.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	go Run(ctx, Config{Enabled: true, Welcome: "Welcome, {name}!"}, f, priv, nil)
	time.Sleep(35 * time.Second) // first look: whoever is already on isn't welcomed
	f.mu.Lock()
	f.players = append(f.players, palworld.Player{Name: "Moss", UserID: "steam_2"})
	f.mu.Unlock()
	time.Sleep(32 * time.Second)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.private) != 1 || f.private[0] != "steam_2:Welcome, Moss!" {
		t.Fatalf("only the newcomer is welcomed, privately: %v", f.private)
	}
}
