package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const taskName = "Palsforge Warden"

// subcommand handles "warden install|uninstall|open|remote ...". It returns false when the
// arguments aren't a subcommand (then Warden runs normally).
func subcommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	var err error
	switch args[0] {
	case "install":
		err = install(args[1:])
	case "uninstall":
		err = uninstall()
	case "open":
		err = openPanel(listenFrom(args[1:]))
	case "remote":
		err = remote(args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		return false
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "warden:", err)
		os.Exit(1)
	}
	return true
}

const usage = `Palsforge Warden: an admin panel for a Palworld dedicated server.

  warden -server <folder>            run Warden (panel on http://127.0.0.1:8282)
  warden install -server <folder>    start Warden with Windows (a background task)
  warden uninstall                   stop starting it with Windows
  warden open                        open the panel in its own window
  warden remote on|off               let remote admins sign in over your Tailscale network (never the internet)

Options: -data <folder> (Warden's own data), -listen 127.0.0.1:<port> (local, no sign-in),
-admin-listen 127.0.0.1:<port> (remote admins, sign-in), -check.
`

func listenFrom(args []string) string {
	for i, a := range args {
		if (a == "-listen" || a == "--listen") && i+1 < len(args) {
			return args[i+1]
		}
	}
	return "127.0.0.1:8282"
}

func flagValue(args []string, name string) string {
	for i, a := range args {
		if (a == "-"+name || a == "--"+name) && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, "-"+name+"="); ok {
			return v
		}
	}
	return ""
}

func install(args []string) error {
	server := flagValue(args, "server")
	if server == "" {
		server = findServer()
	}
	if server == "" {
		return errors.New("say where the server is: warden install -server <folder>")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	run := []string{"-server", server}
	if d := flagValue(args, "data"); d != "" {
		run = append(run, "-data", d)
	}
	if l := flagValue(args, "listen"); l != "" {
		run = append(run, "-listen", l)
	}
	if runtime.GOOS != "windows" {
		fmt.Printf(`On Linux, run Warden as a systemd service. Save this as /etc/systemd/system/warden.service,
then: systemctl enable --now warden

[Unit]
Description=Palsforge Warden
After=network-online.target

[Service]
ExecStart=%s %s
# Warden restarts itself by exiting after a settings change; systemd starts it again.
Restart=always
RestartSec=2
# Only Warden is stopped when it restarts, never the game server it supervises.
KillMode=process
User=%s

[Install]
WantedBy=multi-user.target
`, exe, strings.Join(run, " "), os.Getenv("USER"))
		return nil
	}
	quoted := make([]string, len(run))
	for i, a := range run {
		quoted[i] = `"` + a + `"`
	}
	tr := fmt.Sprintf(`"%s" %s`, exe, strings.Join(quoted, " "))
	out, err := exec.Command("schtasks", "/Create", "/F", "/TN", taskName, "/SC", "ONSTART", "/RU", "SYSTEM", "/RL", "HIGHEST", "/TR", tr).CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't create the startup task (run this from an Administrator window): %s", strings.TrimSpace(string(out)))
	}
	exec.Command("schtasks", "/Run", "/TN", taskName).Run()
	if err := startMenuShortcut(exe); err == nil {
		fmt.Println("Warden will now start with Windows, and it's starting now.\nOpen the panel from the Start menu: \"Palsforge Warden\".")
	} else {
		fmt.Printf("Warden will now start with Windows, and it's starting now.\nOpen the panel with: \"%s\" open\n", exe)
	}
	return nil
}

func uninstall() error {
	if runtime.GOOS != "windows" {
		fmt.Println("On Linux: systemctl disable --now warden")
		return nil
	}
	exec.Command("schtasks", "/End", "/TN", taskName).Run()
	out, err := exec.Command("schtasks", "/Delete", "/F", "/TN", taskName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't remove the startup task: %s", strings.TrimSpace(string(out)))
	}
	os.Remove(shortcutPath())
	fmt.Println("Warden no longer starts with Windows. Its data and backups are kept.")
	return nil
}

// openPanel shows the panel in its own window: Edge or Chrome in app mode when available,
// otherwise the default browser.
func openPanel(listen string) error {
	url := "http://" + listen + "/"
	if runtime.GOOS == "windows" {
		for _, b := range []string{
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		} {
			if _, err := os.Stat(b); err == nil {
				return exec.Command(b, "--app="+url, "--window-size=1280,860").Start()
			}
		}
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}

// remote publishes the panel to the admin's own tailnet with "tailscale serve" (HTTPS, tailnet
// only), or stops publishing it.
func remote(args []string) error {
	if len(args) == 0 || (args[0] != "on" && args[0] != "off") {
		return errors.New("use: warden remote on   or   warden remote off")
	}
	ts := "tailscale"
	if runtime.GOOS == "windows" {
		if p := `C:\Program Files\Tailscale\tailscale.exe`; fileExists(p) {
			ts = p
		}
	}
	if _, err := exec.LookPath(ts); err != nil && !fileExists(ts) {
		return errors.New("Tailscale isn't installed: install it from tailscale.com and sign in, then run this again")
	}
	var cmd *exec.Cmd
	if args[0] == "on" {
		admin := flagValue(args[1:], "admin-listen")
		if admin == "" {
			admin = "127.0.0.1:8283"
		}
		cmd = exec.Command(ts, "serve", "--bg", "--https=8443", "http://"+admin) // the sign-in port, never the local one
	} else {
		cmd = exec.Command(ts, "serve", "--https=8443", "off")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale serve failed: %s", strings.TrimSpace(string(out)))
	}
	if args[0] == "on" {
		fmt.Println("The panel is now on your Tailscale network only (not the internet), at port 8443:")
		fmt.Println(strings.TrimSpace(string(out)))
	} else {
		fmt.Println("The panel is no longer shared on your Tailscale network.")
	}
	return nil
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// findServer looks for a Palworld server in the usual places.
func findServer() string {
	var c []string
	if wd, err := os.Getwd(); err == nil {
		c = append(c, wd)
	}
	if exe, err := os.Executable(); err == nil {
		c = append(c, filepath.Dir(exe))
	}
	if runtime.GOOS == "windows" {
		for _, d := range []string{"C", "D", "E"} {
			c = append(c, d+`:\PalServer`, d+`:\Palworld`, d+`:\steamcmd\steamapps\common\PalServer`,
				d+`:\Program Files (x86)\Steam\steamapps\common\PalServer`, d+`:\SteamLibrary\steamapps\common\PalServer`)
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		c = append(c, filepath.Join(home, "Steam/steamapps/common/PalServer"), filepath.Join(home, ".steam/steam/steamapps/common/PalServer"), "/home/steam/palworld", "/palworld")
	}
	for _, d := range c {
		if fileExists(filepath.Join(d, "Pal")) && (fileExists(filepath.Join(d, "PalServer.exe")) || fileExists(filepath.Join(d, "PalServer.sh")) || fileExists(filepath.Join(d, "Pal", "Binaries"))) {
			return d
		}
	}
	return ""
}

// pauseIfDoubleClicked keeps the window open long enough to read a message when Warden was
// started by double-clicking (it has its own console then).
func pauseIfDoubleClicked() {
	if runtime.GOOS == "windows" && os.Getenv("PROMPT") == "" {
		fmt.Println("\nPress Enter to close.")
		done := make(chan struct{})
		go func() { fmt.Scanln(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Minute):
		}
	}
}

// restartSelf starts a fresh copy of Warden with the same arguments and exits, so new settings
// apply exactly as at startup.
func restartSelf() {
	if os.Getenv("INVOCATION_ID") != "" { // under systemd: exit and let it restart Warden (KillMode=process keeps the game running)
		fmt.Fprintln(os.Stderr, "warden: settings changed; restarting")
		os.Exit(3)
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "warden: couldn't restart:", err)
		return
	}
	fmt.Fprintln(os.Stderr, "warden: settings changed; restarted as process", cmd.Process.Pid)
	os.Exit(0)
}

func shortcutPath() string {
	return filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`, "Palsforge Warden.lnk")
}

// startMenuShortcut adds "Palsforge Warden" to every user's Start menu; it opens the panel.
func startMenuShortcut(exe string) error {
	ps := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut('%s'); $s.TargetPath='%s'; $s.Arguments='open'; $s.Description='Palsforge Warden: the Palworld server panel'; $s.Save()`,
		strings.ReplaceAll(shortcutPath(), "'", "''"), strings.ReplaceAll(exe, "'", "''"))
	return exec.Command("powershell", "-NoProfile", "-Command", ps).Run()
}

// firstRun is what a double-click does (no arguments): a short guided setup that finds the
// server, installs Warden to start with Windows (asking Windows for permission once), and opens
// the panel. On other systems it explains the command to run.
func firstRun() {
	fmt.Println("Palsforge Warden setup")
	fmt.Println("======================")
	exe, _ := os.Executable()
	if runtime.GOOS != "windows" {
		fmt.Println("Run:  warden install -server /path/to/PalServer   (it prints a systemd unit to install)")
		return
	}
	server := findServer()
	in := bufio.NewReader(os.Stdin)
	if server != "" {
		fmt.Printf("\nFound your Palworld server at:\n  %s\nIs that right? [Y/n] ", server)
		if a, _ := in.ReadString('\n'); strings.HasPrefix(strings.ToLower(strings.TrimSpace(a)), "n") {
			server = ""
		}
	}
	for server == "" {
		fmt.Print("\nWhere is your Palworld server folder (the one with PalServer.exe)? ")
		a, _ := in.ReadString('\n')
		a = strings.Trim(strings.TrimSpace(a), `"`)
		if fileExists(filepath.Join(a, "Pal")) {
			server = a
		} else if a != "" {
			fmt.Println("That folder has no Pal folder inside; try again.")
		}
	}
	fmt.Println("\nWarden will start with Windows and look after this server. Windows will ask for permission once.")
	fmt.Print("Install now? [Y/n] ")
	if a, _ := in.ReadString('\n'); strings.HasPrefix(strings.ToLower(strings.TrimSpace(a)), "n") {
		fmt.Println("Nothing was installed.")
		pauseIfDoubleClicked()
		return
	}
	ps := fmt.Sprintf(`Start-Process -FilePath '%s' -ArgumentList 'install','-server','"%s"' -Verb RunAs -Wait -WindowStyle Hidden`,
		strings.ReplaceAll(exe, "'", "''"), strings.ReplaceAll(server, "'", "''"))
	if err := exec.Command("powershell", "-NoProfile", "-Command", ps).Run(); err != nil {
		fmt.Println("Windows didn't give permission, so nothing was installed. Run warden.exe again to retry.")
		pauseIfDoubleClicked()
		return
	}
	fmt.Println("\nInstalled. Opening the panel... (next time, use \"Palsforge Warden\" in the Start menu)")
	time.Sleep(3 * time.Second)
	openPanel("127.0.0.1:8282")
	time.Sleep(2 * time.Second)
}
