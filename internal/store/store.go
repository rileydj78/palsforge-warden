// Package store keeps Warden's own data in one SQLite file: admins, the audit log, the history
// of server metrics and every player ever seen.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
		if _, err := db.Exec(q); err != nil {
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
}

// AddSample stores one history point.
func (s *Store) AddSample(p Sample) {
	s.DB.Exec(`INSERT OR REPLACE INTO samples(ts,online,players,fps,frametime) VALUES(?,?,?,?,?)`,
		p.TS, p.Online, p.Players, p.FPS, p.FrameTime)
}

// History returns samples since a time, and drops ones older than 30 days.
func (s *Store) History(since time.Time) ([]Sample, error) {
	s.DB.Exec(`DELETE FROM samples WHERE ts < ?`, time.Now().Add(-30*24*time.Hour).Unix())
	rows, err := s.DB.Query(`SELECT ts,online,COALESCE(players,0),COALESCE(fps,0),COALESCE(frametime,0) FROM samples WHERE ts >= ? ORDER BY ts`, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Sample{}
	for rows.Next() {
		var p Sample
		rows.Scan(&p.TS, &p.Online, &p.Players, &p.FPS, &p.FrameTime)
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
