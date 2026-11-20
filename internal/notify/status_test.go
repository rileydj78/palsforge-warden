package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestLiveStatusPostsOnceThenEdits(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.Method == http.MethodPost {
			json.NewEncoder(w).Encode(map[string]string{"id": "777"})
		}
	}))
	defer s.Close()
	l := &LiveStatus{Webhook: s.URL + "/api/webhooks/1/tok", DataDir: t.TempDir()}
	info := StatusInfo{Online: true, Name: "Palsforge Anarchy", Players: []string{"Moss"}, Max: 32, FPS: 60}
	l.Update(context.Background(), info)
	l.Update(context.Background(), info) // unchanged: nothing sent
	info.Players = append(info.Players, "Fern")
	l.Update(context.Background(), info)
	got := strings.Join(calls, "|")
	if got != "POST /api/webhooks/1/tok|PATCH /api/webhooks/1/tok/messages/777" {
		t.Fatalf("calls: %s", got)
	}
}
