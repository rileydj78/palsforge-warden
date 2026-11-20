package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// API keys let a bot or website use the administration port without a browser: sent as
// "Authorization: Bearer wk_...". A key acts as a moderator or an admin, never the owner, so it
// can't manage accounts or Warden's settings. Only a SHA-256 of each key is stored (keys are 32
// random bytes, so a fast hash is enough); the key itself is shown once.

const keyPrefix = "wk_"

func keyHash(tok string) string { h := sha256.Sum256([]byte(tok)); return hex.EncodeToString(h[:]) }

// NewKey makes a key and returns it (the only time it's ever shown).
func (a *Auth) NewKey(name, role, by string) (string, error) {
	if !validName(name) {
		return "", errors.New("key names are 2-32 letters, digits, dots, dashes or underscores")
	}
	if role != Moderator && role != Admin {
		return "", errors.New("a key is a moderator or an admin (never the owner)")
	}
	tok := keyPrefix + token(32)
	_, err := a.DB.Exec(`INSERT INTO api_keys(name, hash, role, created, by) VALUES(?,?,?,?,?)`, name, keyHash(tok), role, time.Now().Unix(), by)
	if err != nil {
		return "", errors.New("there's already a key with that name")
	}
	return tok, nil
}

// KeySession returns a session for a valid key, or nil. Its user name is "key:<name>".
func (a *Auth) KeySession(tok string) *Session {
	if !strings.HasPrefix(tok, keyPrefix) || len(tok) > 100 {
		return nil
	}
	var name, role string
	if a.DB.QueryRow(`SELECT name, role FROM api_keys WHERE hash=?`, keyHash(tok)).Scan(&name, &role) != nil {
		return nil
	}
	if role == Owner { // never, whatever the database says
		return nil
	}
	a.DB.Exec(`UPDATE api_keys SET last_used=? WHERE name=?`, time.Now().Unix(), name)
	return &Session{User: User{Name: "key:" + name, Role: role}, Key: true, Expires: time.Now().Add(time.Minute)}
}

// Keys lists the keys (never the keys themselves).
func (a *Auth) Keys() ([]map[string]any, error) {
	rows, err := a.DB.Query(`SELECT name, role, created, COALESCE(last_used,0), COALESCE(by,'') FROM api_keys ORDER BY created`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var n, r, by string
		var c, l int64
		rows.Scan(&n, &r, &c, &l, &by)
		out = append(out, map[string]any{"name": n, "role": r, "created": c, "lastUsed": l, "by": by})
	}
	return out, rows.Err()
}

// RevokeKey deletes a key at once.
func (a *Auth) RevokeKey(name string) error {
	res, err := a.DB.Exec(`DELETE FROM api_keys WHERE name=?`, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("no such key")
	}
	return nil
}
