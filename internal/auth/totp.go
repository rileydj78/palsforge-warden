package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Two-step sign-in with an authenticator app (TOTP, RFC 6238: 30-second steps, 6 digits, SHA-1,
// the settings every authenticator app understands).

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func totpAt(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, secret)
	m.Write(msg[:])
	h := m.Sum(nil)
	off := h[len(h)-1] & 0x0f
	code := (binary.BigEndian.Uint32(h[off:off+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", code)
}

// totpOK accepts the current code and the ones either side (clocks drift).
func totpOK(secretB32, code string, now time.Time) bool {
	secret, err := b32.DecodeString(strings.ToUpper(secretB32))
	if err != nil || len(code) != 6 {
		return false
	}
	step := now.Unix() / 30
	for d := int64(-1); d <= 1; d++ {
		if subtle.ConstantTimeCompare([]byte(totpAt(secret, step+d)), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// StartTOTP makes a new secret for an account (not active until ConfirmTOTP). It returns the key
// to type into an app and the otpauth:// link.
func (a *Auth) StartTOTP(name, issuer string) (key, link string, err error) {
	raw := make([]byte, 20)
	rand.Read(raw)
	key = b32.EncodeToString(raw)
	if _, err := a.DB.Exec(`UPDATE users SET totp_pending=? WHERE name=?`, key, name); err != nil {
		return "", "", err
	}
	link = fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s", url.PathEscape(issuer), url.PathEscape(name), key, url.QueryEscape(issuer))
	return key, link, nil
}

// ConfirmTOTP turns two-step sign-in on once the app's first code matches.
func (a *Auth) ConfirmTOTP(name, code string) error {
	var pending string
	if err := a.DB.QueryRow(`SELECT COALESCE(totp_pending,'') FROM users WHERE name=?`, name).Scan(&pending); err != nil || pending == "" {
		return errors.New("start two-step sign-in first")
	}
	if !totpOK(pending, strings.TrimSpace(code), time.Now()) {
		return errors.New("that code doesn't match; check the time on your phone and try the newest code")
	}
	_, err := a.DB.Exec(`UPDATE users SET totp=?, totp_pending=NULL WHERE name=?`, pending, name)
	return err
}

// DisableTOTP turns two-step sign-in off for an account (the owner can do this for anyone who
// lost their phone).
func (a *Auth) DisableTOTP(name string) error {
	_, err := a.DB.Exec(`UPDATE users SET totp=NULL, totp_pending=NULL WHERE name=?`, name)
	return err
}

func (a *Auth) totpSecret(name string) string {
	var s string
	a.DB.QueryRow(`SELECT COALESCE(totp,'') FROM users WHERE name=?`, name).Scan(&s)
	return s
}
