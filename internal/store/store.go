// Package store keeps Warden's own data in one SQLite file: admins, the audit log, the history
// of server metrics and every player ever seen.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure Go: warden stays one file, no C compiler
)

// Store is the database.
type Store struct{ DB *sql.DB }

var schema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		name TEXT PRIMARY KEY, hash TEXT NOT NULL, role TEXT NOT NULL,
		created INTEGER NOT NULL, last_login INTEGER)`,
	`CREATE TABLE IF NOT EXISTS audit (
		id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL, who TEXT NOT NULL,
		action TEXT NOT NULL, target TEXT, detail TEXT, ok INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS samples (
		ts INTEGER PRIMARY KEY, online INTEGER NOT NULL, players INTEGER, fps REAL, frametime REAL)`,
	`ALTER TABLE users ADD COLUMN totp TEXT`,
	`ALTER TABLE samples ADD COLUMN cpu REAL`,
	`CREATE TABLE IF NOT EXISTS kits (user_id TEXT PRIMARY KEY, given INTEGER)`,
	`CREATE TABLE IF NOT EXISTS allowed (user_id TEXT PRIMARY KEY, added INTEGER, by TEXT)`,
	`ALTER TABLE samples ADD COLUMN mem REAL`,
	`ALTER TABLE users ADD COLUMN totp_pending TEXT`,
	`CREATE TABLE IF NOT EXISTS runs (id INTEGER PRIMARY KEY, kind TEXT, started INTEGER, ok INTEGER, report TEXT)`,
	`CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY, ts INTEGER, kind TEXT, text TEXT)`,
	`CREATE TABLE IF NOT EXISTS players (
		user_id TEXT PRIMARY KEY, name TEXT NOT NULL, platform TEXT, level INTEGER,
		first_seen INTEGER NOT NULL, last_seen INTEGER NOT NULL, seconds INTEGER NOT NULL DEFAULT 0,
		banned INTEGER NOT NULL DEFAULT 0, note TEXT)`,
	`CREATE INDEX IF NOT EXISTS audit_ts ON audit(ts)`,
}

// DefaultDir is where Warden keeps its data when -data is not given.
func DefaultDir() string {
	if pd := os.Getenv("ProgramData"); pd != "" {
		return filepath.Join(pd, "Warden")
	}
	return "/var/lib/warden"
}

// Open opens (and creates) the database in dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "warden.db")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: one writer; keeps it simple and safe
	for _, q := range schema {
		if _, err := db.Exec(q); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return nil, fmt.Errorf("database setup: %w", err)
		}
	}
	return &Store{DB: db}, nil
}

// Audit records one admin action.
func (s *Store) Audit(who, action, target, detail string, ok bool) {
	s.DB.Exec(`INSERT INTO audit(ts,who,action,target,detail,ok) VALUES(?,?,?,?,?,?)`,
		time.Now().Unix(), who, action, target, detail, ok)
}

// AuditEntry is one line of the audit log.
type AuditEntry struct {
	TS     int64  `json:"ts"`
	Who    string `json:"who"`
	Action string `json:"action"`
	Target string `json:"target"`
	Detail string `json:"detail"`
	OK     bool   `json:"ok"`
}

// RecentAudit returns the newest n entries.
func (s *Store) RecentAudit(n int) ([]AuditEntry, error) {
	rows, err := s.DB.Query(`SELECT ts,who,action,COALESCE(target,''),COALESCE(detail,''),ok FROM audit ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		rows.Scan(&e.TS, &e.Who, &e.Action, &e.Target, &e.Detail, &e.OK)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Sample is one point of history.
type Sample struct {
	TS        int64   `json:"ts"`
	Online    bool    `json:"online"`
	Players   int     `json:"players"`
	FPS       float64 `json:"fps"`
	FrameTime float64 `json:"frametime"`
	CPU       float64 `json:"cpu"` // the machine's CPU use, %
	Mem       float64 `json:"mem"` // the machine's memory use, %
}

// AddSample stores one history point.
func (s *Store) AddSample(p Sample) {
	s.DB.Exec(`INSERT OR REPLACE INTO samples(ts,online,players,fps,frametime,cpu,mem) VALUES(?,?,?,?,?,?,?)`,
		p.TS, p.Online, p.Players, p.FPS, p.FrameTime, p.CPU, p.Mem)
}

// History returns samples since a time, and drops ones older than 30 days.
func (s *Store) History(since time.Time) ([]Sample, error) {
	s.DB.Exec(`DELETE FROM samples WHERE ts < ?`, time.Now().Add(-30*24*time.Hour).Unix())
	rows, err := s.DB.Query(`SELECT ts,online,COALESCE(players,0),COALESCE(fps,0),COALESCE(frametime,0),COALESCE(cpu,0),COALESCE(mem,0) FROM samples WHERE ts >= ? ORDER BY ts`, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Sample{}
	for rows.Next() {
		var p Sample
		rows.Scan(&p.TS, &p.Online, &p.Players, &p.FPS, &p.FrameTime, &p.CPU, &p.Mem)
		out = append(out, p)
	}
	return out, rows.Err()
}

// SeenPlayer is one player in the roster.
type SeenPlayer struct {
	UserID    string `json:"userId"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	Level     int    `json:"level"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
	Seconds   int64  `json:"seconds"`
	Banned    bool   `json:"banned"`
	Note      string `json:"note"`
}

// SawPlayer records that a player was online at ts, adding elapsed seconds since the last sighting
// when it is recent enough to count as the same session.
func (s *Store) SawPlayer(userID, name, platform string, level int, ts int64, gap int64) {
	var last int64
	err := s.DB.QueryRow(`SELECT last_seen FROM players WHERE user_id=?`, userID).Scan(&last)
	if err == sql.ErrNoRows {
		s.DB.Exec(`INSERT INTO players(user_id,name,platform,level,first_seen,last_seen,seconds) VALUES(?,?,?,?,?,?,0)`,
			userID, name, platform, level, ts, ts)
		return
	}
	add := int64(0)
	if d := ts - last; d > 0 && d <= gap {
		add = d
	}
	s.DB.Exec(`UPDATE players SET name=?,platform=?,level=?,last_seen=?,seconds=seconds+? WHERE user_id=?`,
		name, platform, level, ts, add, userID)
}

// SetBanned marks a player banned or not in the roster.
func (s *Store) SetBanned(userID string, banned bool) {
	s.DB.Exec(`UPDATE players SET banned=? WHERE user_id=?`, banned, userID)
}

// Roster returns everyone seen, most recent first.
func (s *Store) Roster() ([]SeenPlayer, error) {
	rows, err := s.DB.Query(`SELECT user_id,name,COALESCE(platform,''),COALESCE(level,0),first_seen,last_seen,seconds,banned,COALESCE(note,'') FROM players ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SeenPlayer{}
	for rows.Next() {
		var p SeenPlayer
		rows.Scan(&p.UserID, &p.Name, &p.Platform, &p.Level, &p.FirstSeen, &p.LastSeen, &p.Seconds, &p.Banned, &p.Note)
		out = append(out, p)
	}
	return out, rows.Err()
}

// Run is one nightly routine (or a manual run), with its JSON report.
type Run struct {
	ID      int64           `json:"id"`
	Kind    string          `json:"kind"`
	Started int64           `json:"started"`
	OK      bool            `json:"ok"`
	Report  json.RawMessage `json:"report"`
}

// AddRun stores a routine report.
func (s *Store) AddRun(kind string, started time.Time, ok bool, report any) {
	b, _ := json.Marshal(report)
	s.DB.Exec(`INSERT INTO runs(kind, started, ok, report) VALUES(?,?,?,?)`, kind, started.Unix(), ok, string(b))
	s.DB.Exec(`DELETE FROM runs WHERE id NOT IN (SELECT id FROM runs ORDER BY id DESC LIMIT 200)`)
}

// Runs returns the newest n runs.
func (s *Store) Runs(n int) ([]Run, error) {
	rows, err := s.DB.Query(`SELECT id, kind, started, ok, report FROM runs ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		var rep string
		if err := rows.Scan(&r.ID, &r.Kind, &r.Started, &r.OK, &rep); err == nil {
			r.Report = json.RawMessage(rep)
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

// Event is a server event: a crash, a restart, an update.
type Event struct {
	TS   int64  `json:"ts"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// AddEvent records a server event (keeps the newest 1000).
func (s *Store) AddEvent(ts time.Time, kind, text string) {
	s.DB.Exec(`INSERT INTO events(ts, kind, text) VALUES(?,?,?)`, ts.Unix(), kind, text)
	s.DB.Exec(`DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT 1000)`)
}

// Events returns the newest n events.
func (s *Store) Events(n int) ([]Event, error) {
	rows, err := s.DB.Query(`SELECT ts, kind, text FROM events ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.TS, &e.Kind, &e.Text); err == nil {
			out = append(out, e)
		}
	}
	return out, rows.Err()
}

// SetNote stores an admin's note about a player.
func (s *Store) SetNote(userID, note string) error {
	res, err := s.DB.Exec(`UPDATE players SET note=? WHERE user_id=?`, note, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no player %s in the roster yet", userID)
	}
	return nil
}

// Allow puts a player on the allow-list (or takes them off).
func (s *Store) Allow(userID, by string, on bool) error {
	if on {
		_, err := s.DB.Exec(`INSERT OR REPLACE INTO allowed(user_id, added, by) VALUES(?,?,?)`, userID, time.Now().Unix(), by)
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM allowed WHERE user_id=?`, userID)
	return err
}

// Allowed says whether a player is on the allow-list.
func (s *Store) Allowed(userID string) bool {
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM allowed WHERE user_id=?`, userID).Scan(&n)
	return n > 0
}

// AllowList returns everyone on the allow-list, with the name Warden last saw them under.
func (s *Store) AllowList() ([]map[string]any, error) {
	rows, err := s.DB.Query(`SELECT a.user_id, COALESCE(p.name,''), a.added, COALESCE(a.by,'') FROM allowed a LEFT JOIN players p ON p.user_id=a.user_id ORDER BY a.added`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, by string
		var added int64
		rows.Scan(&id, &name, &added, &by)
		out = append(out, map[string]any{"userId": id, "name": name, "added": added, "by": by})
	}
	return out, rows.Err()
}

// KitGiven says whether a player already had their starter kit.
func (s *Store) KitGiven(userID string) bool {
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM kits WHERE user_id=?`, userID).Scan(&n)
	return n > 0
}

// MarkKit records that a player got their starter kit.
func (s *Store) MarkKit(userID string) {
	s.DB.Exec(`INSERT OR IGNORE INTO kits(user_id, given) VALUES(?,?)`, userID, time.Now().Unix())
}

// Known says whether Warden has seen a player before now (in the roster).
func (s *Store) Known(userID string) bool {
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM players WHERE user_id=?`, userID).Scan(&n)
	return n > 0
}
