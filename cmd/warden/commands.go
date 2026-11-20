package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/auth"
	"github.com/rileydj78/palsforge-warden/internal/backup"
	"github.com/rileydj78/palsforge-warden/internal/config"
	setup "github.com/rileydj78/palsforge-warden/internal/install"
	"github.com/rileydj78/palsforge-warden/internal/instances"
	"github.com/rileydj78/palsforge-warden/internal/store"
	"golang.org/x/crypto/ssh"
)

// taskNameFor is the Windows task for the main Warden ("") or a named instance.
func taskNameFor(name string) string {
	if name == "" {
		return "Palsforge Warden"
	}
	return "Palsforge Warden (" + name + ")"
}

// instanceAddrs returns a named instance's chosen ports ("" name: the defaults).
func instanceAddrs(name string) (local, admin string) {
	if name == "" {
		return "127.0.0.1:8282", "127.0.0.1:8283"
	}
	p := loadPorts(instanceData(name))
	return p.Local, p.Admin
}

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
		err = uninstall(flagValue(args[1:], "name"))
	case "open":
		l := listenFrom(args[1:])
		if n := flagValue(args[1:], "name"); n != "" && l == "127.0.0.1:8282" {
			l, _ = instanceAddrs(n)
		}
		err = openPanel(l)
	case "remote":
		err = remote(args[1:])
	case "install-server":
		_, err = installServer(args[1:])
	case "sftp-setup":
		err = sftpSetup(args[1:])
	case "adduser":
		err = addUserCmd(args[1:])
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
                                     add -name <name> for another server on the same machine
  warden uninstall                   stop starting it with Windows
  warden open                        open the panel in its own window
  warden install-server <folder>     download SteamCMD and the Palworld server, with first settings Warden can use
  warden sftp-setup sftp://user@host/folder   make a key for backups on another machine and print what to add
  warden adduser <name> <role>       make a remote admin account (moderator, admin or owner); reads the password from input
  warden remote public on|off        put the read-only status page on the internet (Tailscale Funnel)
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
	name := flagValue(args, "name")
	if name != "" {
		if err := instances.ValidName(name); err != nil {
			return err
		}
		run = append(run, "-name", name)
	}
	taskName := taskNameFor(name)
	if d := flagValue(args, "data"); d != "" {
		run = append(run, "-data", d)
	}
	if l := flagValue(args, "listen"); l != "" {
		run = append(run, "-listen", l)
	}
	if runtime.GOOS != "windows" {
		unit := "warden"
		if name != "" {
			unit = "warden-" + name
		}
		fmt.Printf(`On Linux, run Warden as a systemd service. Save this as /etc/systemd/system/`+unit+`.service,
then: systemctl enable --now `+unit+`

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
	if err := startMenuShortcut(exe, name); err == nil {
		fmt.Printf("Warden will now start with Windows, and it's starting now.\nOpen the panel from the Start menu: %q.\n", taskName)
	} else {
		fmt.Printf("Warden will now start with Windows, and it's starting now.\nOpen the panel with: \"%s\" open\n", exe)
	}
	return nil
}

func uninstall(name string) error {
	taskName := taskNameFor(name)
	if runtime.GOOS != "windows" {
		fmt.Println("On Linux: systemctl disable --now warden")
		return nil
	}
	exec.Command("schtasks", "/End", "/TN", taskName).Run()
	out, err := exec.Command("schtasks", "/Delete", "/F", "/TN", taskName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't remove the startup task: %s", strings.TrimSpace(string(out)))
	}
	os.Remove(shortcutPath(name))
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
	public := len(args) > 0 && args[0] == "public"
	if public {
		args = args[1:]
	}
	if len(args) == 0 || (args[0] != "on" && args[0] != "off") {
		return errors.New("use: warden remote on|off   or   warden remote public on|off")
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
	if public {
		var cmd *exec.Cmd
		if args[0] == "on" {
			pub := flagValue(args[1:], "public-listen")
			if pub == "" {
				pub = "127.0.0.1:8284"
			}
			// Funnel publishes to the internet: only the status-page port, which has no other routes.
			cmd = exec.Command(ts, "funnel", "--bg", "--https=443", "http://"+pub)
		} else {
			cmd = exec.Command(ts, "funnel", "--https=443", "off")
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("tailscale funnel failed (Funnel must be allowed for this machine in Tailscale's admin console): %s", strings.TrimSpace(string(out)))
		}
		if args[0] == "on" {
			fmt.Println("The status page is now public (read-only: name, online, player count). Turn it on in Warden's Settings too.")
			fmt.Println(strings.TrimSpace(string(out)))
		} else {
			fmt.Println("The status page is no longer public.")
		}
		return nil
	}
	name := flagValue(args[1:], "name")
	_, admin := instanceAddrs(name)
	if a := flagValue(args[1:], "admin-listen"); a != "" {
		admin = a
	}
	if admin == "" {
		return errors.New("that instance hasn't run yet, so it has no ports; start it once first")
	}
	// The main Warden is on 8443; each instance gets its own port (8444, 8445, ...) from its
	// administration port, so they don't move.
	httpsPort := 8443
	if _, p, err := net.SplitHostPort(admin); err == nil {
		if n, _ := strconv.Atoi(p); n > 8283 {
			httpsPort = 8443 + (n-8283)/10
		}
	}
	hp := fmt.Sprintf("--https=%d", httpsPort)
	var cmd *exec.Cmd
	if args[0] == "on" {
		cmd = exec.Command(ts, "serve", "--bg", hp, "http://"+admin) // the sign-in port, never the local one
	} else {
		cmd = exec.Command(ts, "serve", hp, "off")
	}
	data := store.DefaultDir()
	if name != "" {
		data = instanceData(name)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale serve failed: %s", strings.TrimSpace(string(out)))
	}
	if args[0] == "on" {
		fmt.Printf("The panel is now on your Tailscale network only (not the internet), at port %d:\n", httpsPort)
		fmt.Println(strings.TrimSpace(string(out)))
		if m := regexp.MustCompile(`https://\S+`).FindString(string(out)); m != "" {
			os.WriteFile(filepath.Join(data, "remote-url.txt"), []byte(m), 0o600) // for the server switcher
		}
	} else {
		fmt.Println("The panel is no longer shared on your Tailscale network.")
		os.Remove(filepath.Join(data, "remote-url.txt"))
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

func shortcutPath(name string) string {
	return filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`, taskNameFor(name)+".lnk")
}

// startMenuShortcut adds "Palsforge Warden" to every user's Start menu; it opens the panel.
func startMenuShortcut(exe, name string) error {
	arg := "open"
	if name != "" {
		arg = "open -name " + name
	}
	ps := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut('%s'); $s.TargetPath='%s'; $s.Arguments='%s'; $s.Description='Palsforge Warden: the Palworld server panel'; $s.Save()`,
		strings.ReplaceAll(shortcutPath(name), "'", "''"), strings.ReplaceAll(exe, "'", "''"), arg)
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
		fmt.Print("\nWhere is your Palworld server folder (the one with PalServer.exe)?\n(No server yet? Type NEW and Warden downloads one into C:\\PalServer.) ")
		a, _ := in.ReadString('\n')
		a = strings.Trim(strings.TrimSpace(a), `"`)
		if strings.EqualFold(a, "new") {
			dir, err := installServer([]string{`C:\PalServer`})
			if err != nil {
				fmt.Println("That didn't work:", err)
				continue
			}
			server = dir
			continue
		}
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

// addUserCmd makes an account from the command line (for Docker and headless Linux, where there
// is no browser on the machine).
func addUserCmd(args []string) error {
	var pos []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			i++ // a flag's value
			continue
		}
		pos = append(pos, args[i])
	}
	if len(pos) != 2 {
		return errors.New("use: warden adduser <name> <moderator|admin|owner> [-data <folder>], then type the password")
	}
	dir := flagValue(args, "data")
	if dir == "" {
		dir = store.DefaultDir()
	}
	st, err := store.Open(dir)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	au, err := auth.New(st.DB)
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stderr, "Password (at least 10 characters): ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return errors.New("no password given")
	}
	if err := au.AddUser(pos[0], strings.TrimRight(line, "\r\n"), pos[1]); err != nil {
		return err
	}
	st.Audit("command line", "add admin", pos[0], pos[1], true)
	fmt.Fprintf(os.Stderr, "Made %s (%s). They can sign in now.\n", pos[0], pos[1])
	return nil
}

// installServer sets up a Palworld server from nothing and tells Warden where SteamCMD is.
func installServer(args []string) (string, error) {
	var dir string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			i++
			continue
		}
		dir = args[i]
	}
	if dir == "" {
		return "", errors.New("use: warden install-server <folder> [-name \"My server\"]")
	}
	dir, _ = filepath.Abs(dir)
	if fileExists(filepath.Join(dir, "Pal")) {
		fmt.Println("There's already a server in", dir, "- Warden will update it instead of starting over.")
	}
	steamDir := filepath.Join(filepath.Dir(dir), "steamcmd")
	fmt.Println("Getting SteamCMD (Valve's downloader) into", steamDir, "...")
	ctx := context.Background()
	steam, err := setup.SteamCMD(ctx, steamDir)
	if err != nil {
		return "", err
	}
	fmt.Println("Downloading the Palworld dedicated server into", dir, "(about 6 GB; SteamCMD first updates itself) ...")
	last := time.Time{}
	err = setup.Server(ctx, steam, dir, func(line string) {
		if strings.Contains(line, "progress:") && time.Since(last) < 5*time.Second {
			return // one progress line every few seconds is plenty
		}
		last = time.Now()
		fmt.Println("  ", line)
	})
	if err != nil {
		return "", err
	}
	name := flagValue(args, "name")
	if name == "" {
		name = "My Palworld server"
	}
	ini, err := setup.StarterSettings(dir, name)
	if err != nil {
		return "", err
	}
	fmt.Println("Settings written to", ini, "(REST API on, a random admin password Warden reads itself).")
	data := flagValue(args, "data")
	if data == "" {
		data = store.DefaultDir()
	}
	if w, found, err := config.Load(data, dir); err == nil && !found {
		w.ServerDir = dir
		w.Update.SteamCMD = steam
		os.MkdirAll(data, 0o700)
		config.Save(data, w)
	}
	fmt.Println("\nThe server is installed. Keep the game's ports open for players (UDP 8211, and 27015 for the")
	fmt.Println("server list) and keep TCP 8212 (the REST API) closed to the internet.")
	return dir, nil
}

// sftpSetup makes Warden's SSH key (once), reads the other machine's host key, and prints the
// lines to add there and in Warden's settings.
func sftpSetup(args []string) error {
	if len(args) == 0 {
		return errors.New("use: warden sftp-setup sftp://user@host/folder [-data <folder>]")
	}
	u, err := url.Parse(args[0])
	if err != nil || u.Scheme != "sftp" || u.Host == "" || u.User == nil {
		return errors.New("the address looks like sftp://user@host/folder")
	}
	data := flagValue(args, "data")
	if data == "" {
		data = store.DefaultDir()
	}
	keyFile := filepath.Join(data, "sftp_ed25519")
	if _, err := os.Stat(keyFile); err != nil {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		block, err := ssh.MarshalPrivateKey(priv, "palsforge-warden")
		if err != nil {
			return err
		}
		if err := os.WriteFile(keyFile, pem.EncodeToMemory(block), 0o600); err != nil {
			return err
		}
		sp, _ := ssh.NewPublicKey(pub)
		os.WriteFile(keyFile+".pub", ssh.MarshalAuthorizedKey(sp), 0o644)
	}
	pubLine, err := os.ReadFile(keyFile + ".pub")
	if err != nil {
		return err
	}
	hp := u.Host
	if u.Port() == "" {
		hp = net.JoinHostPort(u.Hostname(), "22")
	}
	fp, typ, err := backup.HostKeyOf(hp)
	if err != nil {
		return fmt.Errorf("couldn't reach %s: %w", hp, err)
	}
	fmt.Printf("1. On %s, add this line to %s's ~/.ssh/authorized_keys (restrict,command=\"internal-sftp\" lets the key do SFTP and nothing else):\n\n   restrict,command=\"internal-sftp\" %s\n\n", u.Hostname(), u.User.Username(), strings.TrimSpace(string(pubLine)))
	fmt.Printf("2. Check this is really %s's %s host key (on that machine: ssh-keygen -lf /etc/ssh/ssh_host_*_key.pub):\n\n   %s\n\n", u.Hostname(), typ, fp)
	fmt.Printf("3. In warden.json, under backups, add:\n\n   \"sftp\": [{\"url\": %q, \"keyFile\": %q, \"hostKey\": %q}]\n", args[0], keyFile, fp)
	return nil
}
