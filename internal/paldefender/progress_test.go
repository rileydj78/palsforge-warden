package paldefender

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProgressionTechsEggs(t *testing.T) {
	var path string
	var body map[string]any
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.Method + " " + r.URL.Path
		b, _ := io.ReadAll(r.Body)
		body = nil
		json.Unmarshal(b, &body)
		if strings.Contains(r.URL.Path, "offline") {
			w.WriteHeader(400)
			w.Write([]byte(`{"Error":{"Code":"REQUEST_FAILED","Message":"Failed to find APalPlayerController by UserId or PlayerUID 'offline'."}}`))
			return
		}
		if r.Method == "GET" {
			w.Write([]byte(`{"Meta":{"UnlockedCount":2,"LockedCount":5,"TotalCount":7},"Techs":{"Unlocked":["Technology_B","Technology_A"]}}`))
			return
		}
		w.Write([]byte(`{"Granted":{"EXP":500},"Totals":{}}`))
	}))
	defer s.Close()
	c := New(Config{URL: s.URL, Token: "t"})
	ctx := context.Background()
	if _, err := c.GiveProgression(ctx, "steam_1", Progression{}); err == nil {
		t.Fatal("an empty grant must not be sent")
	}
	if _, err := c.GiveProgression(ctx, "steam_1", Progression{Relics: map[string]int{"Wings": 1}}); err == nil {
		t.Fatal("unknown relic types must be refused")
	}
	r, err := c.GiveProgression(ctx, "steam_1", Progression{EXP: 500, Relics: map[string]int{"MoveSpeed": 2}})
	if err != nil || path != "POST /v1/pdapi/give/progression/steam_1" || body["EXP"] != float64(500) || body["TechnologyPoints"] != nil || body["Relics"].(map[string]any)["MoveSpeed"] != float64(2) || r["Granted"] == nil {
		t.Fatalf("%v %s %v %v", err, path, body, r)
	}
	un, locked, total, err := c.Techs(ctx, "steam_1")
	if err != nil || len(un) != 2 || un[0] != "Technology_A" || locked != 5 || total != 7 {
		t.Fatal(un, locked, total, err)
	}
	if _, err := c.SetTechs(ctx, "steam_1", true, true, nil); err != nil || path != "POST /v1/pdapi/learntech/steam_1" || body["Technology"] != "All" {
		t.Fatalf("learn all: %v %s %v", err, path, body)
	}
	if _, err := c.SetTechs(ctx, "steam_1", false, false, []string{"Technology_A"}); err != nil || path != "POST /v1/pdapi/forgettech/steam_1" || body["Technology"].([]any)[0] != "Technology_A" {
		t.Fatalf("forget one: %v %s %v", err, path, body)
	}
	if _, err := c.SetTechs(ctx, "steam_1", true, false, []string{"bad id; drop"}); err == nil {
		t.Fatal("odd technology IDs must be refused")
	}
	if err := c.GiveEgg(ctx, "steam_1", "PalEgg_Fire_01", "Foxparks", 12, 2); err != nil || len(body["PalEggs"].([]any)) != 2 {
		t.Fatalf("eggs: %v %v", err, body)
	}
	if _, _, _, err := c.Techs(ctx, "offline"); err == nil || !strings.Contains(err.Error(), "has to be online") {
		t.Fatalf("offline players get a plain explanation: %v", err)
	}
}
