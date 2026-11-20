// Package palworld talks to a Palworld dedicated server's own REST API (v1).
// The API gives full control of the server, so Warden only ever calls it on 127.0.0.1
// (or the address the admin configured) and keeps the password on the server.
package palworld

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a REST API client for one server.
type Client struct {
	Base     string // e.g. http://127.0.0.1:8212/v1/api
	Password string
	HTTP     *http.Client
}

// New makes a client for host:port with the server's AdminPassword.
func New(host string, port int, password string) *Client {
	return &Client{
		Base:     fmt.Sprintf("http://%s:%d/v1/api", host, port),
		Password: password,
		HTTP:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Info is GET /info.
type Info struct {
	Version     string `json:"version"`
	ServerName  string `json:"servername"`
	Description string `json:"description"`
	WorldGUID   string `json:"worldguid"`
}

// Player is one entry of GET /players.
type Player struct {
	Name        string  `json:"name"`
	AccountName string  `json:"accountName"`
	PlayerID    string  `json:"playerId"`
	UserID      string  `json:"userId"`
	IP          string  `json:"ip"`
	Ping        float64 `json:"ping"`
	X           float64 `json:"location_x"`
	Y           float64 `json:"location_y"`
	Level       int     `json:"level"`
	Buildings   int     `json:"building_count"`
}

// Metrics is GET /metrics.
type Metrics struct {
	ServerFPS       float64 `json:"serverfps"`
	CurrentPlayers  int     `json:"currentplayernum"`
	ServerFrameTime float64 `json:"serverframetime"`
	MaxPlayers      int     `json:"maxplayernum"`
	Uptime          int64   `json:"uptime"`
	Days            int     `json:"days"`
	BaseCampNum     int     `json:"basecampnum"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, rd)
	if err != nil {
		return err
	}
	req.SetBasicAuth("admin", c.Password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("the server's REST API did not answer: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("the server refused the admin password (check AdminPassword)")
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: HTTP %d", method, path, resp.StatusCode)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// Info returns the server's name and version.
func (c *Client) Info(ctx context.Context) (*Info, error) {
	var v Info
	return &v, c.do(ctx, http.MethodGet, "/info", nil, &v)
}

// Players returns who is online.
func (c *Client) Players(ctx context.Context) ([]Player, error) {
	var v struct {
		Players []Player `json:"players"`
	}
	err := c.do(ctx, http.MethodGet, "/players", nil, &v)
	return v.Players, err
}

// Metrics returns FPS, player count and uptime.
func (c *Client) Metrics(ctx context.Context) (*Metrics, error) {
	var v Metrics
	return &v, c.do(ctx, http.MethodGet, "/metrics", nil, &v)
}

// Settings returns the running server's settings (read-only over the API).
func (c *Client) Settings(ctx context.Context) (map[string]any, error) {
	var v map[string]any
	return v, c.do(ctx, http.MethodGet, "/settings", nil, &v)
}
