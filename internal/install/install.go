// Package install sets up a Palworld dedicated server from nothing: SteamCMD, the server itself
// (Steam app 2394010, anonymous login), and a first settings file with the REST API on and a
// random admin password, so Warden can look after it straight away.
package install

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/config"
)

// SteamCMD's official download (Valve).
const steamcmdWindows = "https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip"

// SteamCMD returns the path of steamcmd, downloading it into dir on Windows when it isn't there.
// On Linux it must be installed from the distribution (steamcmd package) or be on PATH.
func SteamCMD(ctx context.Context, dir string) (string, error) {
	if runtime.GOOS != "windows" {
		for _, p := range []string{filepath.Join(dir, "steamcmd.sh"), "/usr/games/steamcmd"} {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		if p, err := exec.LookPath("steamcmd"); err == nil {
			return p, nil
		}
		return "", errors.New("SteamCMD isn't installed: install your distribution's steamcmd package, then run this again")
	}
	exe := filepath.Join(dir, "steamcmd.exe")
	if _, err := os.Stat(exe); err == nil {
		return exe, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", steamcmdWindows, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("couldn't download SteamCMD: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("couldn't download SteamCMD: Valve answered %s", resp.Status)
	}
	zipPath := filepath.Join(dir, "steamcmd.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, 50<<20))
	f.Close()
	if err != nil {
		return "", err
	}
	defer os.Remove(zipPath)
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("the SteamCMD download isn't a zip: %w", err)
	}
	defer z.Close()
	for _, zf := range z.File {
		if zf.Name != "steamcmd.exe" { // the zip holds just this; take nothing else
			continue
		}
		r, err := zf.Open()
		if err != nil {
			return "", err
		}
		out, err := os.Create(exe)
		if err == nil {
			_, err = io.Copy(out, io.LimitReader(r, 50<<20))
			out.Close()
		}
		r.Close()
		if err != nil {
			return "", err
		}
		return exe, nil
	}
	return "", errors.New("the SteamCMD download has no steamcmd.exe in it")
}

// Server downloads (or updates) the Palworld dedicated server into dir. progress gets SteamCMD's
// output lines. A fresh SteamCMD updates itself and exits (code 7) without installing anything,
// so a run that ends that way is repeated.
func Server(ctx context.Context, steamcmd, dir string, progress func(string)) error {
	var err error
	for try := 0; try < 3; try++ {
		var selfUpdated bool
		selfUpdated, err = serverOnce(ctx, steamcmd, dir, progress)
		if err == nil || !selfUpdated {
			return err
		}
		if progress != nil {
			progress("SteamCMD updated itself; running it again to install the server")
		}
	}
	return err
}

func serverOnce(ctx context.Context, steamcmd, dir string, progress func(string)) (bool, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	abs, _ := filepath.Abs(dir)
	ctx, cancel := context.WithTimeout(ctx, 90*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, steamcmd, "+force_install_dir", abs, "+login", "anonymous", "+app_update", config.AppID, "validate", "+quit")
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	var all strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		var line strings.Builder
		for {
			n, err := pr.Read(buf)
			for _, b := range buf[:n] {
				if b == '\n' || b == '\r' {
					if s := strings.TrimSpace(line.String()); s != "" && progress != nil {
						progress(s)
					}
					all.WriteString(line.String() + "\n")
					line.Reset()
					continue
				}
				line.WriteByte(b)
			}
			if err != nil {
				return
			}
		}
	}()
	err := cmd.Run()
	pw.Close()
	<-done
	ok := strings.Contains(all.String(), "Success! App '"+config.AppID+"'") || strings.Contains(all.String(), "already up to date")
	if !ok {
		selfUpdated := strings.Contains(all.String(), "Update complete, launching")
		if err == nil {
			err = errors.New("SteamCMD didn't report success")
		}
		return selfUpdated, fmt.Errorf("installing the server failed: %w", err)
	}
	return false, nil
}

var (
	reREST  = regexp.MustCompile(`RESTAPIEnabled=\w+`)
	reAdmin = regexp.MustCompile(`AdminPassword="[^"]*"`)
	reName  = regexp.MustCompile(`ServerName="[^"]*"`)
)

// StarterSettings writes PalWorldSettings.ini from the server's DefaultPalWorldSettings.ini, with
// the REST API on (bound by the game to all addresses: keep port 8212 closed in the firewall) and
// a random admin password Warden reads from the file. It never overwrites an existing file.
func StarterSettings(serverDir, name string) (string, error) {
	dst := config.SettingsPath(serverDir)
	if b, err := os.ReadFile(dst); err == nil && strings.Contains(string(b), "OptionSettings=") {
		return dst, nil // already set up: leave it as it is
	}
	def, err := os.ReadFile(filepath.Join(serverDir, "DefaultPalWorldSettings.ini"))
	if err != nil {
		return "", fmt.Errorf("the server has no DefaultPalWorldSettings.ini (is the install complete?): %w", err)
	}
	text := string(def)
	if !strings.Contains(text, "OptionSettings=") {
		return "", errors.New("DefaultPalWorldSettings.ini doesn't look like Palworld's settings")
	}
	raw := make([]byte, 18)
	rand.Read(raw)
	pw := strings.NewReplacer("+", "", "/", "", "=", "").Replace(base64.StdEncoding.EncodeToString(raw))
	text = reREST.ReplaceAllString(text, "RESTAPIEnabled=True")
	text = reAdmin.ReplaceAllString(text, `AdminPassword="`+pw+`"`)
	if name = strings.Map(func(r rune) rune {
		if r == '"' || r == ',' || r == '(' || r == ')' || r < 32 {
			return -1
		}
		return r
	}, name); name != "" {
		text = reName.ReplaceAllString(text, `ServerName="`+name+`"`)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	return dst, os.WriteFile(dst, []byte(text), 0o600)
}
