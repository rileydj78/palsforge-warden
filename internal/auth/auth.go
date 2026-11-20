// Package auth: admin accounts, sign-in and sessions.
//
//   - Passwords are hashed with Argon2id; only the hash is stored.
//   - There is no first-run account: whoever is at the server machine uses the local port as the
//     owner without signing in. Accounts are for remote admins; the owner creates them.
//   - Sessions are random tokens in an HttpOnly, SameSite=Strict cookie; every change also needs
//     the session's CSRF token in a header.
//   - Five wrong passwords in a row lock sign-in for a minute.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// Roles, weakest first.
const (
	Moderator = "moderator" // see everything; announce, kick
	Admin     = "admin"     // also ban, save, restart, backups, settings
	Owner     = "owner"     // also manage admins
)

var rank = map[string]int{Moderator: 1, Admin: 2, Owner: 3}

// Allows reports whether role may do what needs at least min.
func Allows(role, min string) bool { return rank[role] >= rank[min] }

// User is a signed-in admin.
type User struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// Session is one signed-in browser.
type Session struct {
	User    User
	CSRF    string
	Expires time.Time
	Key     bool // an API key, not a signed-in person
}

// Auth manages accounts and sessions.
type Auth struct {
	DB       *sql.DB
	mu       sync.Mutex
	sessions map[string]*Session
	attempts map[string]*attempt
	codeLock map[string]*codeLock
}

// SessionLife is how long a sign-in lasts.
const SessionLife = 12 * time.Hour

// New prepares auth.
func New(db *sql.DB) (*Auth, error) {
	a := &Auth{DB: db, sessions: map[string]*Session{}}
	if _, err := a.UserCount(); err != nil { // the database is readable
		return nil, err
	}
	return a, nil
}

func token(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(b), "=")
}

// UserCount is how many admins exist.
func (a *Auth) UserCount() (int, error) {
	var n int
	return n, a.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
}

func hash(pw string, salt []byte) string {
	k := argon2.IDKey([]byte(pw), salt, 2, 64*1024, 2, 32)
	return fmt.Sprintf("argon2id$2$65536$2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(k))
}

func verify(pw, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "argon2id" {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, 2, 64*1024, 2, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// CheckPassword returns an error explaining why a password is not good enough, or nil.
func CheckPassword(pw string) error {
	if len([]rune(pw)) < 10 {
		return errors.New("use at least 10 characters")
	}
	return nil
}

func validName(n string) bool {
	if len(n) < 2 || len(n) > 32 {
		return false
	}
	for _, r := range n {
		if !(r == '_' || r == '-' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// NewToken returns a random token (for the local port's session).
func NewToken(n int) string { return token(n) }

// RemoveUser deletes an account and ends its sessions.
func (a *Auth) RemoveUser(name string) error {
	res, err := a.DB.Exec(`DELETE FROM users WHERE name=?`, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("no such account")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for t, ses := range a.sessions {
		if ses.User.Name == name {
			delete(a.sessions, t)
		}
	}
	return nil
}

// AddUser makes an account for a remote admin (the owner does this).
func (a *Auth) AddUser(name, pw, role string) error {
	if role != Admin && role != Moderator && role != Owner {
		return errors.New("role must be moderator, admin or owner")
	}
	return a.addUser(name, pw, role)
}

func (a *Auth) addUser(name, pw, role string) error {
	if !validName(name) {
		return errors.New("names are 2-32 letters, digits, dots, dashes or underscores")
	}
	if err := CheckPassword(pw); err != nil {
		return err
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	_, err := a.DB.Exec(`INSERT INTO users(name,hash,role,created) VALUES(?,?,?,?)`, name, hash(pw, salt), role, time.Now().Unix())
	if err != nil {
		return errors.New("that name is already taken")
	}
	return nil
}

// Login checks a password and starts a session. Returns the session token.
func (a *Auth) Login(name, pw string) (string, *Session, error) { return a.LoginFrom(name, pw, "", "") }

// ErrNeedCode means the password was right and the account also needs its two-step code.
var ErrNeedCode = errors.New("enter the 6-digit code from your authenticator app")

// verifySlots limits how many Argon2id checks (64 MB each) run at once.
var verifySlots = make(chan struct{}, 3)

// LoginFrom checks a password. Failures are counted per name and per source address before the
// slow check runs, so parallel guesses can't slip past the limit, and one attacker can't lock the
// owner out from everywhere.
func (a *Auth) LoginFrom(name, pw, source, code string) (string, *Session, error) {
	keys := []string{"name:" + strings.ToLower(name), "src:" + source}
	a.mu.Lock()
	if a.attempts == nil {
		a.attempts = map[string]*attempt{}
	}
	now := time.Now()
	for _, k := range keys {
		at := a.attempts[k]
		if at == nil {
			at = &attempt{}
			a.attempts[k] = at
		}
		if now.Before(at.until) {
			a.mu.Unlock()
			return "", nil, errors.New("too many wrong passwords; wait a minute")
		}
		if at.n >= 5 { // five wrong already: lock for a minute
			at.n, at.until = 0, now.Add(time.Minute)
			a.mu.Unlock()
			return "", nil, errors.New("too many wrong passwords; wait a minute")
		}
	}
	for _, k := range keys {
		a.attempts[k].n++ // reserve the attempt before the slow check; a success takes it back
	}
	if len(a.attempts) > 10000 { // forget old entries
		for k, at := range a.attempts {
			if now.After(at.until) && at.n == 0 {
				delete(a.attempts, k)
			}
		}
	}
	a.mu.Unlock()
	verifySlots <- struct{}{}
	var stored, role string
	err := a.DB.QueryRow(`SELECT hash, role FROM users WHERE name=?`, name).Scan(&stored, &role)
	ok := err == nil && verify(pw, stored)
	if err != nil {
		verify(pw, hash("x", make([]byte, 16))) // same work either way: no hint whether the name exists
	}
	<-verifySlots
	a.mu.Lock()
	defer a.mu.Unlock()
	if !ok {
		return "", nil, errors.New("that name and password don't match")
	}
	if secret := a.totpSecret(name); secret != "" {
		if code == "" {
			return "", nil, ErrNeedCode
		}
		key := strings.ToLower(name)
		if a.codeLock == nil {
			a.codeLock = map[string]*codeLock{}
		}
		cl := a.codeLock[key]
		if cl == nil {
			cl = &codeLock{}
			a.codeLock[key] = cl
		}
		if time.Now().Before(cl.until) {
			return "", nil, errors.New("too many wrong two-step codes; this account is locked for a while (the owner can turn two-step off for it)")
		}
		step, ok := totpStep(secret, code, time.Now())
		if !ok || step <= cl.lastStep { // wrong, or already used once
			cl.fails++
			switch {
			case cl.fails >= 10:
				cl.until = time.Now().Add(24 * time.Hour)
			case cl.fails >= 5:
				cl.until = time.Now().Add(time.Hour)
			}
			return "", nil, errors.New("that two-step code isn't right (each code works once)")
		}
		cl.fails, cl.lastStep = 0, step
	}
	for _, k := range keys {
		if at := a.attempts[k]; at != nil {
			at.n = 0
		}
	}
	tok := token(32)
	s := &Session{User: User{Name: name, Role: role}, CSRF: token(24), Expires: time.Now().Add(SessionLife)}
	for t, old := range a.sessions {
		if time.Now().After(old.Expires) {
			delete(a.sessions, t)
		}
	}
	a.sessions[tok] = s
	a.DB.Exec(`UPDATE users SET last_login=? WHERE name=?`, time.Now().Unix(), name)
	return tok, s, nil
}

type codeLock struct {
	fails    int
	until    time.Time
	lastStep int64 // the last accepted 30-second step: a code works once
}

type attempt struct {
	n     int
	until time.Time
}

// Session returns the live session for a token, or nil.
func (a *Auth) Session(tok string) *Session {
	if tok == "" {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessions[tok]
	if s == nil || time.Now().After(s.Expires) {
		delete(a.sessions, tok)
		return nil
	}
	return s
}

// Logout ends a session.
func (a *Auth) Logout(tok string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, tok)
}

// Users lists the admins.
func (a *Auth) Users() ([]map[string]any, error) {
	rows, err := a.DB.Query(`SELECT name, role, created, COALESCE(last_login,0), COALESCE(totp,'')<>'' FROM users ORDER BY created`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var n, r string
		var c, l int64
		var two bool
		rows.Scan(&n, &r, &c, &l, &two)
		out = append(out, map[string]any{"name": n, "role": r, "created": c, "lastLogin": l, "twoStep": two})
	}
	return out, rows.Err()
}
