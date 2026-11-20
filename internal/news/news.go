// Package news reads Pocketpair's Palworld announcements from Steam's public news feed (no key,
// no identifiers sent). A patch announcement tells Warden to look for the new server build more
// often, and the admin hears about it straight away. Only used when the admin turns on "update
// soon".
package news

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Palworld's Steam app (the game; the dedicated server app has no news).
const feed = "https://api.steampowered.com/ISteamNews/GetNewsForApp/v2/?appid=1623730&count=6&feeds=steam_community_announcements"

// Item is one announcement.
type Item struct {
	ID    string    `json:"id"`
	Title string    `json:"title"`
	URL   string    `json:"url"`
	Date  time.Time `json:"date"`
	Patch bool      `json:"patch"`
}

var patchRe = regexp.MustCompile(`(?i)^v\d+\.\d+|hot ?fix|patch|maintenance|\bupdate\b`)

// IsPatch says whether a title announces a game update (not a bundle or a poll).
func IsPatch(title string) bool {
	return patchRe.MatchString(title) && !strings.Contains(strings.ToLower(title), "bundle")
}

// Feed fetches and remembers the latest announcements.
type Feed struct {
	DataDir string
	URL     string // for tests; "" = Steam
	mu      sync.Mutex
	items   []Item
	at      time.Time
}

// Latest returns what was last fetched.
func (f *Feed) Latest() []Item {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Item(nil), f.items...)
}

// Fetch reads the feed and returns announcements Warden hasn't seen before (remembered across
// restarts, so nobody is told twice).
func (f *Feed) Fetch(ctx context.Context) (fresh []Item, err error) {
	u := f.URL
	if u == "" {
		u = feed
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "Palsforge-Warden")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Steam news: %s", resp.Status)
	}
	var d struct {
		AppNews struct {
			Items []struct {
				GID   string `json:"gid"`
				Title string `json:"title"`
				URL   string `json:"url"`
				Date  int64  `json:"date"`
			} `json:"newsitems"`
		} `json:"appnews"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&d); err != nil {
		return nil, err
	}
	var items []Item
	for _, n := range d.AppNews.Items {
		if !strings.HasPrefix(n.URL, "https://") {
			n.URL = ""
		}
		items = append(items, Item{ID: n.GID, Title: n.Title, URL: n.URL, Date: time.Unix(n.Date, 0), Patch: IsPatch(n.Title)})
	}
	seen := f.loadSeen()
	first := len(seen) == 0 // the first time, everything is "seen": no flood of old news
	for _, it := range items {
		if !seen[it.ID] && !first {
			fresh = append(fresh, it)
		}
		seen[it.ID] = true
	}
	f.mu.Lock()
	f.items, f.at = items, time.Now()
	f.mu.Unlock()
	if err := f.saveSeen(seen); err != nil {
		return fresh, fmt.Errorf("can't remember which news was seen (%v): new posts may be announced again", err)
	}
	return fresh, nil
}

// PatchSince says whether a patch was announced after t.
func (f *Feed) PatchSince(t time.Time) bool {
	for _, it := range f.Latest() {
		if it.Patch && it.Date.After(t) {
			return true
		}
	}
	return false
}

func (f *Feed) seenPath() string { return filepath.Join(f.DataDir, "news-seen.json") }

func (f *Feed) loadSeen() map[string]bool {
	m := map[string]bool{}
	if b, err := os.ReadFile(f.seenPath()); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func (f *Feed) saveSeen(m map[string]bool) error {
	if len(m) > 200 { // keep it small; old ids never come back in a 6-item feed (items are current)
		m = map[string]bool{}
		for _, it := range f.Latest() {
			m[it.ID] = true
		}
	}
	b, _ := json.Marshal(m)
	return os.WriteFile(f.seenPath(), b, 0o600)
}
