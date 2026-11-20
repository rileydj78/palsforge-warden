package palworld

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fake(t *testing.T) (*Client, func()) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "admin" || p != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/api/info":
			fmt.Fprint(w, `{"version":"v1.0.5","servername":"Test","description":"","worldguid":"ABC"}`)
		case "/v1/api/players":
			fmt.Fprint(w, `{"players":[{"name":"Mossbeard","accountName":"moss","playerId":"1","userId":"steam_1","ip":"10.0.0.5","ping":24.5,"location_x":1,"location_y":2,"level":48,"building_count":3}]}`)
		case "/v1/api/metrics":
			fmt.Fprint(w, `{"serverfps":59,"currentplayernum":1,"serverframetime":16.9,"maxplayernum":32,"uptime":3600,"days":4,"basecampnum":2}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	host, port := "", 0
	fmt.Sscanf(strings.TrimPrefix(srv.URL, "http://"), "127.0.0.1:%d", &port)
	host = "127.0.0.1"
	return New(host, port, "pw"), srv.Close
}

func TestClient(t *testing.T) {
	c, done := fake(t)
	defer done()
	ctx := context.Background()
	info, err := c.Info(ctx)
	if err != nil || info.Version != "v1.0.5" {
		t.Fatalf("info: %v %+v", err, info)
	}
	ps, err := c.Players(ctx)
	if err != nil || len(ps) != 1 || ps[0].Name != "Mossbeard" || ps[0].Level != 48 {
		t.Fatalf("players: %v %+v", err, ps)
	}
	m, err := c.Metrics(ctx)
	if err != nil || m.ServerFPS != 59 || m.MaxPlayers != 32 {
		t.Fatalf("metrics: %v %+v", err, m)
	}
	c.Password = "wrong"
	if _, err := c.Info(ctx); err == nil || !strings.Contains(err.Error(), "refused the admin password") {
		t.Fatalf("expected a password error, got %v", err)
	}
}
