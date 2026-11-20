// Command warden is Palsforge Warden: an admin tool for a Palworld dedicated server.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/config"
	"github.com/rileydj78/palsforge-warden/internal/palworld"
	"github.com/rileydj78/palsforge-warden/internal/web"
)

var version = "0.0.1-dev"

func main() {
	serverDir := flag.String("server", "", "the Palworld server folder (the one that holds Pal/ and PalServer)")
	listen := flag.String("listen", "127.0.0.1:8282", "where the panel listens (loopback only)")
	check := flag.Bool("check", false, "check the setup, print what works, and exit")
	flag.Parse()
	if *serverDir == "" {
		fmt.Fprintln(os.Stderr, "warden: say where the server is, e.g. -server C:\\PalServer")
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
	log.Printf("Warden %s: panel on http://%s/ (this machine only)", version, *listen)
	log.Fatal(srv.Listen(*listen))
}
