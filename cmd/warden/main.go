// Command warden is Palsforge Warden: an admin tool for a Palworld dedicated server.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/auth"
	"github.com/rileydj78/palsforge-warden/internal/config"
	"github.com/rileydj78/palsforge-warden/internal/host"
	"github.com/rileydj78/palsforge-warden/internal/monitor"
	"github.com/rileydj78/palsforge-warden/internal/nightly"
	"github.com/rileydj78/palsforge-warden/internal/palworld"
	"github.com/rileydj78/palsforge-warden/internal/store"
	"github.com/rileydj78/palsforge-warden/internal/web"
)

var version = "0.0.1-dev"

func main() {
	if subcommand(os.Args[1:]) {
		return
	}
	serverDir := flag.String("server", "", "the Palworld server folder (the one that holds Pal/ and PalServer)")
	listen := flag.String("listen", "127.0.0.1:8282", "where the panel listens (loopback only)")
	dataDir := flag.String("data", store.DefaultDir(), "where Warden keeps its own data")
	check := flag.Bool("check", false, "check the setup, print what works, and exit")
	flag.Parse()
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
			log.SetOutput(io.MultiWriter(os.Stderr, f)) // a service has no console: the log file is where the setup code is
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
	if au.SetupCode != "" {
		log.Printf("FIRST RUN: open the panel and create the owner account with this setup code: %s", au.SetupCode)
	}
	go monitor.Run(context.Background(), game, st2)

	wcfg, found, err := config.Load(*dataDir, *serverDir)
	if err != nil {
		log.Fatalf("Warden's settings: %v", err)
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
		h := host.New(wcfg, *dataDir)
		h.OnEvent = func(e host.Event) { log.Printf("server: %s", e.Text); st2.AddEvent(e.At, e.Kind, e.Text) }
		runner := &nightly.Runner{Cfg: wcfg, Game: nightly.Game{C: game}, Host: h, Store: st2,
			Backups: &nightly.Backups{Cfg: wcfg, DataDir: *dataDir}}
		srv.Control = &web.Control{Cfg: wcfg, Host: h, Runner: runner}
		go h.Supervise(context.Background())
		go runner.Schedule(context.Background())
		log.Printf("server control: %s mode; nightly routine %s", wcfg.Control.Mode, map[bool]string{true: "at " + wcfg.Nightly.Time, false: "off"}[wcfg.Nightly.Enabled])
	}
	srv.Settings = &web.Settings{DataDir: *dataDir, Restart: restartSelf}
	log.Printf("Warden %s: panel on http://%s/ (this machine only); data in %s", version, *listen, *dataDir)
	log.Fatal(srv.Listen(*listen))
}
