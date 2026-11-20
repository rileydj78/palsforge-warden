// Command warden is Palsforge Warden: an admin tool for a Palworld dedicated server.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/auth"
	"github.com/rileydj78/palsforge-warden/internal/broadcast"
	"github.com/rileydj78/palsforge-warden/internal/config"
	"github.com/rileydj78/palsforge-warden/internal/crash"
	"github.com/rileydj78/palsforge-warden/internal/events"
	"github.com/rileydj78/palsforge-warden/internal/host"
	"github.com/rileydj78/palsforge-warden/internal/instances"
	"github.com/rileydj78/palsforge-warden/internal/kit"
	"github.com/rileydj78/palsforge-warden/internal/mods"
	"github.com/rileydj78/palsforge-warden/internal/monitor"
	"github.com/rileydj78/palsforge-warden/internal/news"
	"github.com/rileydj78/palsforge-warden/internal/nightly"
	"github.com/rileydj78/palsforge-warden/internal/notify"
	"github.com/rileydj78/palsforge-warden/internal/paldefender"
	"github.com/rileydj78/palsforge-warden/internal/palworld"
	"github.com/rileydj78/palsforge-warden/internal/store"
	"github.com/rileydj78/palsforge-warden/internal/web"
)

var version = "0.0.1-dev"

func main() {
	if subcommand(os.Args[1:]) {
		return
	}
	if len(os.Args) == 1 && runtime.GOOS == "windows" { // double-clicked: guided setup
		firstRun()
		return
	}
	serverDir := flag.String("server", "", "the Palworld server folder (the one that holds Pal/ and PalServer)")
	listen := flag.String("listen", "127.0.0.1:8282", "the local panel: this machine only, no sign-in")
	publicListen := flag.String("public-listen", "127.0.0.1:8284", "the read-only status page's port, when it's on in Warden settings (warden remote public on publishes it)")
	adminListen := flag.String("admin-listen", "127.0.0.1:8283", "the administration port for remote admins (sign-in required); warden remote on publishes it")
	dataDir := flag.String("data", store.DefaultDir(), "where Warden keeps its own data")
	check := flag.Bool("check", false, "check the setup, print what works, and exit")
	name := flag.String("name", "", "this Warden's instance name when one machine runs several servers (e.g. pvp); each has its own data and ports")
	flag.Parse()
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if *name != "" {
		if err := instances.ValidName(*name); err != nil {
			log.Fatal(err)
		}
		if !set["data"] {
			*dataDir = instanceData(*name)
		}
		p := loadPorts(*dataDir)
		if p.Local == "" {
			p.Local, p.Admin, p.Public = instances.Ports(store.DefaultDir())
			savePorts(*dataDir, p)
		}
		if !set["listen"] {
			*listen = p.Local
		}
		if !set["admin-listen"] {
			*adminListen = p.Admin
		}
		if !set["public-listen"] {
			*publicListen = p.Public
		}
	}
	if *serverDir == "" {
		*serverDir = findServer()
	}
	if *serverDir == "" {
		fmt.Fprint(os.Stderr, "Warden couldn't find your Palworld server. Say where it is, for example:\n  warden -server C:\\PalServer\n\n"+usage)
		pauseIfDoubleClicked()
		os.Exit(2)
	}
	st, err := config.ReadSettings(config.SettingsPath(*serverDir))
	if err != nil {
		log.Fatalf("could not read the server's settings: %v", err)
	}
	log.Print(st.Describe())
	if !st.RESTEnabled || !st.PasswordIsSet {
		log.Fatal("the server's REST API is off or has no AdminPassword: set RESTAPIEnabled=True and an AdminPassword in PalWorldSettings.ini, then restart the server")
	}
	game := palworld.New("127.0.0.1", st.RESTPort, st.AdminPassword)
	srv := &web.Server{Game: game, Version: version}
	if *check {
		info, err := withTimeout(15*time.Second, game.Info)
		if err != nil {
			fmt.Println("REST API: NOT OK -", err)
			os.Exit(1)
		}
		fmt.Printf("REST API: OK - %s %s\n", info.ServerName, info.Version)
		if m, err := withTimeout(15*time.Second, game.Metrics); err == nil {
			fmt.Printf("Metrics: OK - %d/%d players, %.1f FPS, up %ds\n", m.CurrentPlayers, m.MaxPlayers, m.ServerFPS, m.Uptime)
		}
		return
	}
	if err := os.MkdirAll(*dataDir, 0o700); err == nil {
		if f, err := os.OpenFile(filepath.Join(*dataDir, "warden.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			log.SetOutput(io.MultiWriter(os.Stderr, f)) // a service has no console
		}
	}
	st2, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("could not open Warden's data in %s: %v", *dataDir, err)
	}
	au, err := auth.New(st2.DB)
	if err != nil {
		log.Fatalf("could not read Warden's accounts: %v", err)
	}
	srv.Store, srv.Auth = st2, au
	if n, _ := au.UserCount(); n == 0 {
		log.Printf("no remote accounts yet: on this machine, open the panel (on Windows: Start menu, Palsforge Warden) and add them under Admins")
	}
	go monitor.Run(context.Background(), game, st2)

	for _, d := range []string{*dataDir, store.DefaultDir()} { // secrets, keys, the instance list
		if os.MkdirAll(d, 0o700) == nil {
			protectDir(d)
		}
	}
	wcfg, found, err := config.Load(*dataDir, *serverDir)
	if err != nil {
		log.Fatalf("Warden's settings: %v", err)
	}
	if *name != "" && wcfg.Backups.Tag == "pal" {
		// Each instance names its backups after itself, so instances sharing a backup folder or
		// bucket never count (or delete) each other's archives.
		wcfg.Backups.Tag = instances.Tag(*name)
	}
	if !found {
		wcfg.Nightly.Enabled = false // nothing runs on its own until the admin has looked at the settings
		if err := config.Save(*dataDir, wcfg); err == nil {
			log.Printf("wrote default settings to %s; the nightly routine is OFF until you turn it on there", config.ConfigPath(*dataDir))
		}
	}
	if err := wcfg.Check(); err != nil {
		log.Printf("settings need attention (%s): %v; server control is off", config.ConfigPath(*dataDir), err)
	} else {
		nt := notify.New(wcfg.Notify, st.ServerName)
		var pin func(string) // set once the runner exists
		h := host.New(wcfg, *dataDir)
		h.Alive = func(ctx context.Context) bool { _, err := game.Info(ctx); return err == nil }
		mm := &mods.Manager{ServerDir: wcfg.ServerDir, DataDir: *dataDir}
		h.ModsOn = func() bool {
			l, _, err := mm.List()
			for _, m := range l {
				if m.Enabled {
					return true
				}
			}
			return err != nil // can't tell: assume mods, so a hold stays safe
		}
		h.OnEvent = func(e host.Event) {
			log.Printf("server: %s", e.Text)
			st2.AddEvent(e.At, e.Kind, e.Text)
			switch e.Kind {
			case "crash":
				if pin != nil {
					pin("the last backup before the crash at " + e.At.Format("Jan 2 15:04"))
				}
				msg := e.Text + ". Warden is starting it again."
				if hint := crash.Find(wcfg.ServerDir, crashLogDir(), e.At.Add(-10*time.Minute)); hint.Summary != "" {
					msg += "\n" + hint.Summary
					st2.AddEvent(time.Now(), "crash", "why: "+hint.Summary)
				}
				nt.Send(context.Background(), notify.Urgent, "crash", "The server crashed", msg)
			case "gave-up":
				nt.Send(context.Background(), notify.Urgent, "gave-up", "The server keeps crashing", e.Text)
			case "update":
				nt.Send(context.Background(), notify.Info, "", "Game updated", e.Text)
			}
		}
		runner := &nightly.Runner{Cfg: wcfg, Game: nightly.Game{C: game}, Host: h, Store: st2, Notify: nt,
			Mods:    &mods.Manager{ServerDir: wcfg.ServerDir, DataDir: *dataDir},
			Backups: &nightly.Backups{Cfg: wcfg, DataDir: *dataDir}}
		pin = runner.PinLatest
		go runner.Watch(context.Background())
		go runner.AutoPause(context.Background())
		var feed *news.Feed
		if wcfg.Update.Enabled && wcfg.Update.Soon { // only then does Warden read Steam's news
			feed = &news.Feed{DataDir: *dataDir}
			srv.News = feed
		}
		go runner.UpdateSoon(context.Background(), feed)
		if evs, err := events.Parse(wcfg.Events); err == nil {
			go runner.RunEvents(context.Background(), evs)
		}
		var private broadcast.Private
		if pd := paldefender.New(wcfg.PalDef); pd != nil {
			private = pd.Message
		}
		var seenFn broadcast.Seen
		if wcfg.Notify.Joins && nt.Enabled() {
			seenFn = func(joined, left []string) {
				var parts []string
				if len(joined) > 0 {
					parts = append(parts, "Joined: "+strings.Join(joined, ", "))
				}
				if len(left) > 0 {
					parts = append(parts, "Left: "+strings.Join(left, ", "))
				}
				nt.Send(context.Background(), notify.Info, "", "Players", strings.Join(parts, "\n"))
			}
		}
		if wcfg.Notify.LiveStatus && wcfg.Notify.DiscordWebhook != "" {
			go func() { // the live status message in Discord, refreshed every 2 minutes
				ls := &notify.LiveStatus{Webhook: wcfg.Notify.DiscordWebhook, DataDir: *dataDir}
				for {
					info := notify.StatusInfo{}
					c, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					if in, err := game.Info(c); err == nil {
						info.Online, info.Name, info.Description = true, in.ServerName, in.Description
						if m, err := game.Metrics(c); err == nil {
							info.Max, info.FPS, info.UptimeSecs, info.Day = m.MaxPlayers, m.ServerFPS, int64(m.Uptime), m.Days
						}
						if ps, err := game.Players(c); err == nil {
							for _, p := range ps {
								info.Players = append(info.Players, p.Name)
							}
						}
					} else {
						info.Name = st.ServerName
					}
					if err := ls.Update(c, info); err != nil {
						log.Printf("discord status: %v", err)
					}
					cancel()
					time.Sleep(2 * time.Minute)
				}
			}()
		}
		var gate broadcast.Gate
		if wcfg.AllowList.On {
			msg := wcfg.AllowList.Message
			if msg == "" {
				msg = "This server is invite-only right now."
			}
			gate = func(userID, name string) string {
				if st2.Allowed(userID) {
					return ""
				}
				st2.Audit("allow-list", "kick", name+" ("+userID+")", "not on the allow-list", true)
				return msg
			}
		}
		if pdc := paldefender.New(wcfg.PalDef); wcfg.StarterKit.On && pdc != nil {
			k := &kit.Kit{Cfg: wcfg.StarterKit, Give: pdc, Rec: st2}
			allow := gate
			gate = func(userID, name string) string {
				if allow != nil {
					if msg := allow(userID, name); msg != "" {
						return msg // not let in: no kit
					}
				}
				k.Seen(context.Background(), userID, name)
				return ""
			}
		} else if wcfg.StarterKit.On {
			log.Printf("starter kit is on but PalDefender isn't connected: no kits will be given")
		}
		go broadcast.Run(context.Background(), wcfg.Broadcast, game, private, func() bool { b, _ := runner.Busy(); return b }, seenFn, gate, game.Kick)
		srv.Control = &web.Control{Cfg: wcfg, Host: h, Runner: runner}
		srv.PalDef = paldefender.New(wcfg.PalDef)
		if wcfg.StatusPage.On {
			pub := &web.Public{ShowNames: wcfg.StatusPage.ShowNames, Invite: wcfg.StatusPage.Invite, Address: wcfg.StatusPage.Address}
			for _, l := range strings.Split(wcfg.StatusPage.Rules, "\n") {
				if l = strings.TrimSpace(l); l != "" && len(pub.Rules) < 30 {
					pub.Rules = append(pub.Rules, l)
				}
			}
			if ini, err := config.ReadSettings(config.SettingsPath(wcfg.ServerDir)); err == nil {
				pub.Platforms = strings.ReplaceAll(strings.Trim(ini.All["CrossplayPlatforms"], "()"), ",", ", ")
				pub.Password = strings.Trim(ini.All["ServerPassword"], `"`) != ""
			}
			srv.Public = pub
			log.Printf("status page on http://%s/ (read-only)", *publicListen)
		}
		go h.Supervise(context.Background())
		go runner.Schedule(context.Background())
		log.Printf("server control: %s mode; nightly routine %s", wcfg.Control.Mode, map[bool]string{true: "at " + wcfg.Nightly.Time, false: "off"}[wcfg.Nightly.Enabled])
	}
	srv.Settings = &web.Settings{DataDir: *dataDir, Restart: restartSelf}
	log.Printf("Warden %s: panel on http://%s/ (this machine only, no sign-in); administration port %s (sign-in); data in %s", version, *listen, *adminListen, *dataDir)
	srv.Instances = &web.InstancesInfo{MainData: store.DefaultDir(), Self: *name}
	go func() { // tell the other Wardens on this machine where this one is
		for {
			e := instances.Entry{Name: *name, ServerDir: *serverDir, Local: *listen, Admin: *adminListen, Public: *publicListen, Tag: wcfg.Backups.Tag}
			if b, err := os.ReadFile(filepath.Join(*dataDir, "remote-url.txt")); err == nil {
				e.Remote = strings.TrimSpace(string(b))
			}
			c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if in, err := game.Info(c); err == nil {
				e.ServerName = in.ServerName
			} else {
				e.ServerName = filepath.Base(*serverDir)
			}
			cancel()
			instances.Announce(store.DefaultDir(), e)
			time.Sleep(time.Minute)
		}
	}()
	log.Fatal(srv.Listen(*listen, *adminListen, *publicListen))
}

// instanceData is a named instance's own data folder.
func instanceData(name string) string { return filepath.Join(store.DefaultDir(), "instances", name) }

type ports struct{ Local, Admin, Public string }

// loadPorts returns the ports a named instance chose the first time it ran (kept, so its
// bookmarks and Tailscale address don't move).
func loadPorts(data string) ports {
	var p ports
	if b, err := os.ReadFile(filepath.Join(data, "ports.json")); err == nil {
		json.Unmarshal(b, &p)
	}
	return p
}

func savePorts(data string, p ports) {
	os.MkdirAll(data, 0o700)
	b, _ := json.Marshal(p)
	os.WriteFile(filepath.Join(data, "ports.json"), b, 0o600)
}

// crashLogDir is where UE4SS's log lives under the server folder.
func crashLogDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join("Pal", "Binaries", "Win64")
	}
	return filepath.Join("Pal", "Binaries", "Linux")
}
