package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPTarget is a second place on another machine, reached over SSH with a key. The server's host
// key is pinned: Warden refuses to send backups anywhere else.
type SFTPTarget struct {
	URL     string `json:"url"`     // sftp://user@host[:port]/folder
	KeyFile string `json:"keyFile"` // private key on this machine (warden sftp-setup makes one)
	HostKey string `json:"hostKey"` // the server's key fingerprint, SHA256:...
}

// Check explains what's wrong with a target, or returns nil.
func (t SFTPTarget) Check() error {
	u, err := url.Parse(t.URL)
	if err != nil || u.Scheme != "sftp" || u.Host == "" || u.User == nil || u.User.Username() == "" || u.Path == "" || u.Path == "/" {
		return errors.New("an SFTP place looks like sftp://user@host/folder")
	}
	if _, pw := u.User.Password(); pw {
		return errors.New("SFTP places use a key, never a password in the address")
	}
	if !strings.HasPrefix(t.HostKey, "SHA256:") {
		return errors.New("an SFTP place needs the server's host key (SHA256:...); warden sftp-setup prints it")
	}
	if _, err := os.Stat(t.KeyFile); err != nil {
		return fmt.Errorf("the SFTP key file %s can't be read", t.KeyFile)
	}
	return nil
}

// Name is the place without the user, for messages.
func (t SFTPTarget) Name() string {
	u, err := url.Parse(t.URL)
	if err != nil {
		return "SFTP"
	}
	return "sftp://" + u.Host + u.Path
}

func (t SFTPTarget) dial(ctx context.Context) (*ssh.Client, *sftp.Client, string, error) {
	if err := t.Check(); err != nil {
		return nil, nil, "", err
	}
	u, _ := url.Parse(t.URL)
	key, err := os.ReadFile(t.KeyFile)
	if err != nil {
		return nil, nil, "", err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, nil, "", fmt.Errorf("the SFTP key can't be used: %w", err)
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "22")
	}
	cfg := &ssh.ClientConfig{
		User: u.User.Username(),
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
			if ssh.FingerprintSHA256(k) != t.HostKey {
				return fmt.Errorf("%s's host key is %s, not the one Warden was told to trust; backups were not sent", u.Hostname(), ssh.FingerprintSHA256(k))
			}
			return nil
		},
		HostKeyAlgorithms: hostKeyOrder,
		Timeout:           20 * time.Second,
	}
	d := net.Dialer{Timeout: 20 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, nil, "", err
	}
	// The handshake and SFTP start must finish in 30 s (a server that accepts the connection and
	// then waits, like Tailscale SSH asking for a browser check, would otherwise hold the backup).
	conn.SetDeadline(time.Now().Add(handshakeTimeout))
	c, chans, reqs, err := ssh.NewClientConn(conn, host, cfg)
	if err != nil {
		conn.Close()
		return nil, nil, "", fmt.Errorf("SSH sign-in to %s didn't finish: %w (if it's Tailscale SSH, use the machine's LAN address or turn Tailscale SSH off there)", u.Hostname(), err)
	}
	sc := ssh.NewClient(c, chans, reqs)
	fc, err := sftp.NewClient(sc)
	if err == nil {
		conn.SetDeadline(time.Time{})
		go func() { <-ctx.Done(); sc.Close() }() // a cancelled or timed-out backup ends the transfer
	}
	if err != nil {
		sc.Close()
		return nil, nil, "", fmt.Errorf("the server doesn't offer SFTP: %w", err)
	}
	return sc, fc, u.Path, nil
}

// hostKeyOrder asks for the same host key every time (ED25519 first), so the pinned fingerprint
// is the one people compare against ssh_host_ed25519_key.pub.
// handshakeTimeout bounds the SSH sign-in and SFTP start.
var handshakeTimeout = 30 * time.Second

var hostKeyOrder = []string{ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}

// HostKeyOf connects only far enough to read a server's host key fingerprint and type (for setup).
func HostKeyOf(hostport string) (string, string, error) {
	var fp, typ string
	cfg := &ssh.ClientConfig{User: "warden-probe", Auth: []ssh.AuthMethod{}, Timeout: 15 * time.Second, HostKeyAlgorithms: hostKeyOrder,
		HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
			fp, typ = ssh.FingerprintSHA256(k), k.Type()
			return errors.New("probe")
		}}
	_, err := ssh.Dial("tcp", hostport, cfg)
	if fp != "" {
		return fp, typ, nil
	}
	return "", "", err
}

// SendVerified uploads an archive (as .part, then renamed), reads the copy back and compares its
// SHA-256 with the archive's own.
func (t SFTPTarget) SendVerified(ctx context.Context, a Archive) error {
	sc, fc, dir, err := t.dial(ctx)
	if err != nil {
		return err
	}
	defer sc.Close()
	defer fc.Close()
	if err := fc.MkdirAll(dir); err != nil {
		return fmt.Errorf("can't make %s there: %w", dir, err)
	}
	want, err := fileSHA(a.Path)
	if err != nil {
		return err
	}
	dst := path.Join(dir, a.Name)
	part := dst + ".part"
	src, err := os.Open(a.Path)
	if err != nil {
		return err
	}
	out, err := fc.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		src.Close()
		return err
	}
	_, err = io.Copy(out, src)
	src.Close()
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		fc.Remove(part)
		return fmt.Errorf("sending %s failed: %w", a.Name, err)
	}
	in, err := fc.Open(part)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(h, in)
	in.Close()
	if err != nil || hex.EncodeToString(h.Sum(nil)) != want {
		fc.Remove(part)
		return fmt.Errorf("the copy of %s on %s doesn't match (read back and checked); removed", a.Name, t.Name())
	}
	fc.Remove(dst)
	if err := fc.Rename(part, dst); err != nil {
		return err
	}
	return nil
}

// Mirror deletes this tag's archives there that are no longer kept here, so the other machine
// follows the same retention. It only ever touches files named like Warden's backups.
func (t SFTPTarget) Mirror(ctx context.Context, tag string, keep map[string]bool) (int, error) {
	sc, fc, dir, err := t.dial(ctx)
	if err != nil {
		return 0, err
	}
	defer sc.Close()
	defer fc.Close()
	ents, err := fc.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !nameRe.MatchString(name) || !strings.HasPrefix(name, tag+"-") || keep[name] {
			continue
		}
		if fc.Remove(path.Join(dir, name)) == nil {
			n++
		}
	}
	return n, nil
}

func fileSHA(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
