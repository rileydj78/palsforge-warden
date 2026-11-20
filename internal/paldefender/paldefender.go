// Package paldefender talks to the PalDefender plugin's REST API (docs:
// https://ultimeit.github.io/PalDefender/RESTAPI) and can set that API up for Warden: turned on,
// bound to 127.0.0.1, with a token of Warden's own.
package paldefender

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config is the "palDefender" part of warden.json.
type Config struct {
	URL   string `json:"url"`   // default http://127.0.0.1:17993
	Token string `json:"token"` // Warden's own token (created by Connect)
}

// Dir is PalDefender's folder in a Windows server.
func Dir(serverDir string) string {
	return filepath.Join(serverDir, "Pal", "Binaries", "Win64", "PalDefender")
}

// Installed says whether PalDefender is present.
func Installed(serverDir string) bool {
	_, err := os.Stat(filepath.Join(Dir(serverDir), "RESTAPI"))
	return err == nil
}

// Connect turns PalDefender's REST API on for this machine only and gives Warden a token. It
// keeps every other setting in RESTConfig.json and saves the old file next to it. PalDefender
// reads it at the next server start.
func Connect(serverDir string) (Config, error) {
	dir := filepath.Join(Dir(serverDir), "RESTAPI")
	cfgPath := filepath.Join(dir, "RESTConfig.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return Config{}, fmt.Errorf("PalDefender's REST settings aren't where expected (%s): %w", cfgPath, err)
	}
	var rc map[string]any
	if err := json.Unmarshal(raw, &rc); err != nil {
		return Config{}, fmt.Errorf("RESTConfig.json isn't valid JSON: %w", err)
	}
	port := 17993
	if p, ok := rc["Port"].(float64); ok && p > 0 {
		port = int(p)
	}
	rc["Enabled"] = true
	rc["Address"] = "127.0.0.1" // never the network: Warden is on the same machine
	b, _ := json.MarshalIndent(rc, "", "    ")
	if err := os.WriteFile(cfgPath+".warden-"+time.Now().Format("20060102-150405")+".bak", raw, 0o644); err != nil {
		return Config{}, err
	}
	if err := os.WriteFile(cfgPath, b, 0o644); err != nil {
		return Config{}, err
	}
	tok := make([]byte, 32)
	rand.Read(tok)
	token := hex.EncodeToString(tok)
	tf := map[string]any{"Name": "Warden", "Token": token, "Permissions": []string{"REST.*"}}
	tb, _ := json.MarshalIndent(tf, "", "    ")
	if err := os.MkdirAll(filepath.Join(dir, "Tokens"), 0o755); err != nil {
		return Config{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "Tokens", "Warden.json"), tb, 0o600); err != nil {
		return Config{}, err
	}
	return Config{URL: fmt.Sprintf("http://127.0.0.1:%d", port), Token: token}, nil
}

// Client is a PalDefender REST client.
type Client struct {
	Cfg  Config
	HTTP *http.Client
}

// New returns a client, or nil when PalDefender isn't set up.
func New(cfg Config) *Client {
	if cfg.Token == "" {
		return nil
	}
	if cfg.URL == "" {
		cfg.URL = "http://127.0.0.1:17993"
	}
	return &Client{Cfg: cfg, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Cfg.URL+"/v1/pdapi/"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Cfg.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("PalDefender isn't answering (is its REST API on, and has the server restarted since?)")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errors.New("PalDefender refused Warden's token")
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("PalDefender answered %s: %s", resp.Status, bytes.TrimSpace(b))
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
	}
	return nil
}

// Version is PalDefender's version.
type Version struct {
	Version     string `json:"Version"`
	VersionLong string `json:"VersionLong"`
	Beta        bool   `json:"Beta"`
}

// Version asks PalDefender's version (also a health check).
func (c *Client) Version(ctx context.Context) (*Version, error) {
	var out struct {
		Version Version `json:"Version"`
	}
	if err := c.do(ctx, http.MethodGet, "version", nil, &out); err != nil {
		return nil, err
	}
	return &out.Version, nil
}

// Loc is a position.
type Loc struct{ X, Y, Z float64 }

// Player is PalDefender's view of a player (IP is used only server-side, never sent on).
type Player struct {
	Name        string `json:"Name"`
	IP          string `json:"IP"`
	PlayerUID   string `json:"PlayerUID"`
	UserID      string `json:"UserId"`
	GuildName   string `json:"GuildName"`
	GuildUUID   string `json:"GuildUUID"`
	Status      string `json:"Status"`
	MapLocation Loc    `json:"MapLocation"`
}

// Players lists players PalDefender knows.
func (c *Client) Players(ctx context.Context) ([]Player, error) {
	var out struct {
		Players []Player `json:"Players"`
	}
	err := c.do(ctx, http.MethodGet, "players", nil, &out)
	return out.Players, err
}

// Message sends one player a private chat message.
func (c *Client) Message(ctx context.Context, userID, msg string) error {
	return c.do(ctx, http.MethodPost, "SendPlayerMessage", map[string]any{"SendType": "PlayerChat", "UserID": userID, "Message": msg}, nil)
}

// Ban bans a player, optionally their IP address too.
func (c *Client) Ban(ctx context.Context, userID, reason string, ip bool) error {
	return c.do(ctx, http.MethodPost, "ban/"+url.PathEscape(userID), map[string]any{"Reason": reason, "IP": ip}, nil)
}

// BanEntry is one PalDefender ban, shown as found (its fields vary by version).
type BanEntry map[string]any

// Banlist reads PalDefender's ban list from Banlist.json, where PalDefender keeps it (its API
// answers in another shape, filtered for searches).
func Banlist(_ context.Context, _ *Client, serverDir string) (users, ips []BanEntry, err error) {
	var raw struct {
		UserEntries []BanEntry `json:"UserEntries"`
		IPEntries   []BanEntry `json:"IPEntries"`
	}
	b, err := os.ReadFile(filepath.Join(Dir(serverDir), "Banlist.json"))
	if err != nil {
		return nil, nil, err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, nil, err
	}
	return raw.UserEntries, raw.IPEntries, nil
}

// Guild is a guild summary from PalDefender.
type Guild struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Level int    `json:"Level"`
	Admin struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"admin"`
	CampCount int `json:"camp_count"`
	Camps     []struct {
		ID     string `json:"id"`
		MapPos Loc    `json:"map_pos"`
	} `json:"camps"`
	MemberCount int             `json:"member_count"`
	Members     json.RawMessage `json:"members"`
}

// Guilds lists every guild with its bases.
func (c *Client) Guilds(ctx context.Context) ([]Guild, error) {
	var out struct {
		Guilds map[string]Guild `json:"Guilds"`
	}
	if err := c.do(ctx, http.MethodGet, "guilds", nil, &out); err != nil {
		return nil, err
	}
	list := make([]Guild, 0, len(out.Guilds))
	for id, g := range out.Guilds {
		g.ID = id
		list = append(list, g)
	}
	return list, nil
}

// PlayerDetail is what PalDefender knows about one player: inventory, Pals and progression,
// passed on as PalDefender gives them.
type PlayerDetail struct {
	Inventory   json.RawMessage `json:"inventory,omitempty"`
	Pals        json.RawMessage `json:"pals,omitempty"`
	Progression json.RawMessage `json:"progression,omitempty"`
	Errors      []string        `json:"errors,omitempty"`
}

// Detail fetches a player's inventory, Pals and progression.
func (c *Client) Detail(ctx context.Context, userID string) PlayerDetail {
	var d PlayerDetail
	id := url.PathEscape(userID)
	for _, part := range []struct {
		path, key string
		dst       *json.RawMessage
	}{{"items/" + id, "Inventory", &d.Inventory}, {"pals/" + id, "Pals", &d.Pals}, {"progression/" + id, "Progression", &d.Progression}} {
		var raw map[string]json.RawMessage
		if err := c.do(ctx, http.MethodGet, part.path, nil, &raw); err != nil {
			msg := err.Error()
			if strings.Contains(msg, "APalPlayerController") || strings.Contains(msg, "PLAYER_NOT_FOUND") {
				d.Errors = []string{"PalDefender can only read players who are online right now"}
				return d
			}
			d.Errors = append(d.Errors, msg)
			continue
		}
		*part.dst = raw[part.key]
	}
	return d
}

// GiveItems gives a player items (ItemID → count).
func (c *Client) GiveItems(ctx context.Context, userID, itemID string, count int) error {
	return c.do(ctx, http.MethodPost, "give/items/"+url.PathEscape(userID), map[string]any{"Items": []map[string]any{{"ItemID": itemID, "Count": count}}}, nil)
}

// GivePal gives a player a Pal at a level.
func (c *Client) GivePal(ctx context.Context, userID, palID string, level int) error {
	return c.do(ctx, http.MethodPost, "give/pals/"+url.PathEscape(userID), map[string]any{"Pals": []map[string]any{{"PalID": palID, "Level": level}}}, nil)
}

// Unban lifts a PalDefender user ban.
func (c *Client) Unban(ctx context.Context, userID string) error {
	return c.do(ctx, http.MethodPost, "unban/"+url.PathEscape(userID), map[string]any{"Reason": "unbanned in Warden"}, nil)
}

// UnbanIP lifts a PalDefender IP ban.
func (c *Client) UnbanIP(ctx context.Context, ip string) error {
	return c.do(ctx, http.MethodPost, "unbanip/"+url.PathEscape(ip), map[string]any{"Reason": "unbanned in Warden"}, nil)
}
