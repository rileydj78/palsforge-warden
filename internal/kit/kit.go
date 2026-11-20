// Package kit gives each new player a starter kit once, through PalDefender.
package kit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/config"
)

// Giver is what PalDefender offers.
type Giver interface {
	GiveItems(ctx context.Context, userID, itemID string, count int) error
	GivePal(ctx context.Context, userID, palID string, level int) error
	Message(ctx context.Context, userID, msg string) error
}

// Record remembers who got a kit.
type Record interface {
	KitGiven(userID string) bool
	MarkKit(userID string)
	Audit(user, action, target, detail string, ok bool)
}

// Kit gives the configured kit to players who haven't had it.
type Kit struct {
	Cfg   config.StarterKit
	Give  Giver
	Rec   Record
	mu    sync.Mutex
	tries map[string]int
	first map[string]time.Time
}

// Seen is called for every online player on each look at the server. A player gets the kit once,
// after they've been on for 30 seconds (still loading in before that); a failed give is retried
// on the next looks, at most 5 times. Entries already given aren't given again on a retry.
func (k *Kit) Seen(ctx context.Context, userID, name string) {
	if !k.Cfg.On || k.Give == nil || k.Rec.KitGiven(userID) {
		return
	}
	k.mu.Lock()
	if k.tries == nil {
		k.tries, k.first = map[string]int{}, map[string]time.Time{}
	}
	if _, ok := k.first[userID]; !ok {
		k.first[userID] = time.Now()
	}
	if time.Since(k.first[userID]) < 30*time.Second || k.tries[userID] >= 5 {
		k.mu.Unlock()
		return
	}
	k.tries[userID]++
	k.mu.Unlock()
	items, _ := config.ParseKit(k.Cfg.Items)
	pals, _ := config.ParseKit(k.Cfg.Pals)
	var failed []string
	for _, it := range items {
		n, _ := strconv.Atoi(it[1])
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := k.Give.GiveItems(c, userID, it[0], n); err != nil {
			failed = append(failed, it[0]+": "+err.Error())
		}
		cancel()
	}
	if len(failed) > 0 && len(failed) == len(items) && len(items) > 0 {
		// nothing arrived: try the whole kit again next time
		k.Rec.Audit("starter kit", "give kit", name+" ("+userID+")", "will retry: "+strings.Join(failed, "; "), false)
		return
	}
	for _, p := range pals {
		lv, _ := strconv.Atoi(p[1])
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := k.Give.GivePal(c, userID, p[0], lv); err != nil {
			failed = append(failed, p[0]+": "+err.Error())
		}
		cancel()
	}
	k.Rec.MarkKit(userID) // once given (even partly), never again: no duplicate kits
	detail := fmt.Sprintf("%d items, %d Pals", len(items), len(pals))
	if len(failed) > 0 {
		detail += "; not given: " + strings.Join(failed, "; ")
	}
	k.Rec.Audit("starter kit", "give kit", name+" ("+userID+")", detail, len(failed) == 0)
	if m := strings.TrimSpace(k.Cfg.Message); m != "" {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		k.Give.Message(c, userID, strings.ReplaceAll(m, "{name}", name))
		cancel()
	}
}
