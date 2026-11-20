package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSendsToNtfyAndLimitsRepeats(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r.Header.Get("Title")+"|"+r.Header.Get("Priority")+"|"+string(b))
		mu.Unlock()
	}))
	defer srv.Close()
	n := New(Config{NtfyURL: srv.URL + "/palsforge"}, "Palsforge")
	n.Send(context.Background(), Urgent, "crash", "The server crashed", "It stopped on its own.")
	n.Send(context.Background(), Urgent, "crash", "The server crashed", "again") // limited
	n.Send(context.Background(), Info, "", "Nightly run", "All good.")
	if len(got) != 2 || !strings.HasPrefix(got[0], "Palsforge: The server crashed|urgent|") {
		t.Fatalf("got %q", got)
	}
}

func TestCheck(t *testing.T) {
	if (Config{DiscordWebhook: "https://evil.example/hook"}).Check() == nil {
		t.Fatal("only Discord webhook addresses are accepted")
	}
	if (Config{NtfyURL: "https://ntfy.sh/"}).Check() == nil {
		t.Fatal("an ntfy address needs a topic")
	}
	if (Config{NtfyURL: "https://ntfy.sh/palsforge-alerts", DiscordWebhook: "https://discord.com/api/webhooks/1/abc"}).Check() != nil {
		t.Fatal("valid addresses refused")
	}
	if New(Config{}, "x").Enabled() {
		t.Fatal("nothing configured means nothing is sent")
	}
}
