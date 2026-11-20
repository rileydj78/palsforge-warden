package web

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Public is the read-only status page: its own port, so only these two routes can ever be
// published to the internet (warden remote public on). It never shows IP addresses or IDs, and
// player names only when the owner turns that on.
type Public struct {
	ShowNames bool
	Invite    string // e.g. a Discord invite shown as a link
	Address   string // how to join directly
	Rules     []string
	Platforms string // who can join (CrossplayPlatforms)
	Password  bool   // a join password is set (never the password itself)
	mu        sync.Mutex
	cache     *publicView
	at        time.Time
}

type publicView struct {
	Name, Description, Version, Invite string
	Address, Platforms                 string
	Password                           bool
	Rules                              []string
	Online                             bool
	Players, Max, Day                  int
	Uptime                             string
	Names                              []string
	Checked                            string
}

func (s *Server) publicView(ctx context.Context) *publicView {
	p := s.Public
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cache != nil && time.Since(p.at) < 30*time.Second { // the internet can't make Warden ask the game more often
		return p.cache
	}
	st := s.status(ctx, false)
	v := &publicView{Online: st.Online, Invite: p.Invite, Address: p.Address, Platforms: p.Platforms, Password: p.Password, Rules: p.Rules, Checked: time.Now().UTC().Format("15:04 UTC")}
	if st.Server != nil {
		v.Name, v.Description, v.Version = st.Server.ServerName, st.Server.Description, st.Server.Version
	}
	if m := st.Metrics; m != nil {
		v.Players, v.Max, v.Day = m.CurrentPlayers, m.MaxPlayers, m.Days
		d := time.Duration(m.Uptime) * time.Second
		v.Uptime = (d / time.Minute * time.Minute).String()
		v.Uptime = strings.TrimSuffix(v.Uptime, "0s")
	}
	if p.ShowNames {
		for _, pl := range st.Players {
			v.Names = append(v.Names, pl.Name)
		}
	}
	p.cache, p.at = v, time.Now()
	return v
}

var publicPage = template.Must(template.New("p").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="refresh" content="60"><title>{{if .Name}}{{.Name}}{{else}}Server status{{end}}</title>
<style>
:root{--bg:#F4F6F3;--card:#fff;--ink:#1C2620;--soft:#5B6B60;--line:#DDE3DC;--on:#2F7D4A;--off:#A3261B}
@media (prefers-color-scheme:dark){:root{--bg:#121714;--card:#1B221D;--ink:#E7ECE8;--soft:#9AA89E;--line:#2C362F;--on:#5FBF7E;--off:#E07A6F}}
body{margin:0;background:var(--bg);color:var(--ink);font:16px/1.5 system-ui,-apple-system,Segoe UI,sans-serif}
main{max-width:640px;margin:0 auto;padding:32px 16px}
.card{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:20px;margin-bottom:16px}
h1,h2{font-size:22px;margin:0 0 4px;overflow-wrap:anywhere}.desc{color:var(--soft);margin:0 0 16px;overflow-wrap:anywhere}
.state{font-weight:600}.state::before{content:"";display:inline-block;width:10px;height:10px;border-radius:50%;margin-right:8px;background:var(--off)}
.state.on::before{background:var(--on)}
dl{display:grid;grid-template-columns:repeat(auto-fit,minmax(120px,1fr));gap:12px;margin:16px 0 0}
dt{color:var(--soft);font-size:13px}dd{margin:0;font-size:20px;font-weight:600}
ul,ol{margin:8px 0 0;padding-left:20px}li{margin:4px 0}h2{font-size:18px}a{color:var(--on)}footer{color:var(--soft);font-size:13px}
</style></head><body><main>
<section class="card"><h1>{{if .Name}}{{.Name}}{{else}}Palworld server{{end}}</h1>
{{if .Description}}<p class="desc">{{.Description}}</p>{{end}}
<div class="state{{if .Online}} on{{end}}">{{if .Online}}Online{{else}}Offline right now{{end}}</div>
{{if .Online}}<dl><div><dt>Players</dt><dd>{{.Players}}{{if .Max}} / {{.Max}}{{end}}</dd></div>
{{if .Day}}<div><dt>In-game day</dt><dd>{{.Day}}</dd></div>{{end}}
{{if .Uptime}}<div><dt>Up for</dt><dd>{{.Uptime}}</dd></div>{{end}}
{{if .Version}}<div><dt>Version</dt><dd>{{.Version}}</dd></div>{{end}}</dl>
{{if .Names}}<ul>{{range .Names}}<li>{{.}}</li>{{end}}</ul>{{end}}{{end}}
{{if .Invite}}<p><a href="{{.Invite}}" rel="noopener">Join the community</a></p>{{end}}
</section>
<section class="card"><h2>How to join</h2><ol>
<li>In Palworld: <b>Join Multiplayer Game</b> → <b>Community Servers</b>, and search for <b>{{if .Name}}{{.Name}}{{else}}the server's name{{end}}</b>.</li>
{{if .Address}}<li>Or join directly (PC): enter <b>{{.Address}}</b> at the bottom of that screen.</li>{{end}}
{{if .Password}}<li>It asks for a password: ask in the community{{if .Invite}} (link above){{end}}.</li>{{end}}
</ol>{{if .Platforms}}<p class="desc">Who can join: {{.Platforms}}. Console players join from the Community Servers list.</p>{{end}}</section>
{{if .Rules}}<section class="card"><h2>Rules</h2><ul>{{range .Rules}}<li>{{.}}</li>{{end}}</ul></section>{{end}}<footer>Checked {{.Checked}} · refreshes every minute · Palsforge Warden</footer>
</main></body></html>`))

// PublicHandler serves only the status page and its JSON.
func (s *Server) PublicHandler() http.Handler {
	mux := http.NewServeMux()
	head := func(w http.ResponseWriter) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "public, max-age=30")
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		head(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		publicPage.Execute(w, s.publicView(r.Context()))
	})
	mux.HandleFunc("GET /status.json", func(w http.ResponseWriter, r *http.Request) {
		head(w)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*") // websites and bots may show it
		v := s.publicView(r.Context())
		json.NewEncoder(w).Encode(map[string]any{"name": v.Name, "online": v.Online, "players": v.Players, "maxPlayers": v.Max, "day": v.Day, "version": v.Version, "names": v.Names})
	})
	return mux
}
