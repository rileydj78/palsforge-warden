package paldefender

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// RelicTypes are the relic kinds PalDefender can grant (1.9).
var RelicTypes = []string{"CapturePower", "HungerReduction", "SwimSpeed", "FoodDecayReduction", "JumpPower", "GliderSpeed",
	"ClimbSpeed", "StatusAilmentResist", "StaminaReduction", "SphereHoming", "ExpBonus", "RainbowPassiveRate", "MoveSpeed"}

// Progression is what to grant; zero values are left out.
type Progression struct {
	EXP                     int            `json:"EXP,omitempty"`
	TechnologyPoints        int            `json:"TechnologyPoints,omitempty"`
	AncientTechnologyPoints int            `json:"AncientTechnologyPoints,omitempty"`
	Relics                  map[string]int `json:"Relics,omitempty"`
}

// Empty says nothing would be granted.
func (p Progression) Empty() bool {
	return p.EXP <= 0 && p.TechnologyPoints <= 0 && p.AncientTechnologyPoints <= 0 && len(p.Relics) == 0
}

// GiveProgression grants experience, technology points, ancient technology points and relics.
// The player must be online. The answer carries what was granted and the new totals.
func (c *Client) GiveProgression(ctx context.Context, userID string, p Progression) (map[string]any, error) {
	if p.Empty() {
		return nil, errors.New("nothing to give")
	}
	for k, v := range p.Relics {
		if v <= 0 || !contains(RelicTypes, k) {
			return nil, errors.New("unknown relic type or amount: " + k)
		}
	}
	var out map[string]any
	err := c.do(ctx, http.MethodPost, "give/progression/"+url.PathEscape(userID), p, &out)
	return out, err
}

var techRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,80}$`)

// Techs lists the technologies a player has unlocked.
func (c *Client) Techs(ctx context.Context, userID string) (unlocked []string, locked, total int, err error) {
	var out struct {
		Meta struct {
			Unlocked int `json:"UnlockedCount"`
			Locked   int `json:"LockedCount"`
			Total    int `json:"TotalCount"`
		} `json:"Meta"`
		Techs struct {
			Unlocked []string `json:"Unlocked"`
		} `json:"Techs"`
	}
	if err = c.do(ctx, http.MethodGet, "techs/"+url.PathEscape(userID), nil, &out); err != nil {
		return nil, 0, 0, err
	}
	sort.Strings(out.Techs.Unlocked)
	return out.Techs.Unlocked, out.Meta.Locked, out.Meta.Total, nil
}

// SetTechs learns (learn=true) or forgets technologies: specific IDs, or all of them.
func (c *Client) SetTechs(ctx context.Context, userID string, learn, all bool, ids []string) (map[string]any, error) {
	var tech any = "All"
	if !all {
		if len(ids) == 0 || len(ids) > 200 {
			return nil, errors.New("name 1 to 200 technologies, or all")
		}
		for _, id := range ids {
			if !techRe.MatchString(id) {
				return nil, errors.New("not a technology ID: " + id)
			}
		}
		tech = ids
	}
	path := "forgettech/"
	if learn {
		path = "learntech/"
	}
	var out map[string]any
	err := c.do(ctx, http.MethodPost, path+url.PathEscape(userID), map[string]any{"Technology": tech}, &out)
	return out, err
}

// GiveEgg gives Pal eggs: eggID is the egg item (e.g. PalEgg_Fire_01) holding palID at level.
func (c *Client) GiveEgg(ctx context.Context, userID, eggID, palID string, level, count int) error {
	eggs := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		eggs = append(eggs, map[string]any{"EggID": eggID, "PalID": palID, "Level": level})
	}
	return c.do(ctx, http.MethodPost, "give/paleggs/"+url.PathEscape(userID), map[string]any{"PalEggs": eggs}, nil)
}

// GiveTemplates gives Pals from template files in PalDefender's Pals/Templates folder.
func (c *Client) GiveTemplates(ctx context.Context, userID string, names []string) error {
	return c.do(ctx, http.MethodPost, "give/paltemplate/"+url.PathEscape(userID), map[string]any{"PalTemplates": names}, nil)
}

// Templates lists the Pal template files on this server (PalDefender/Pals/Templates).
func Templates(serverDir string) []string {
	dir := filepath.Join(Dir(serverDir), "Pals", "Templates")
	des, err := os.ReadDir(dir)
	out := []string{}
	if err != nil {
		return out
	}
	for _, d := range des {
		if !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".json") {
			out = append(out, d.Name())
		}
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
