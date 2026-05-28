package db

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

const schema = `
CREATE TABLE IF NOT EXISTS ticks (
    id     INTEGER PRIMARY KEY AUTOINCREMENT,
    ts     INTEGER NOT NULL,
    price  REAL    NOT NULL,
    bid    REAL    NOT NULL,
    ask    REAL    NOT NULL,
    spread REAL    NOT NULL,
    raw    TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ticks_ts ON ticks(ts);
`

// Store wraps a SQLite database for tick persistence.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the SQLite database at path.
func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("WAL mode: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema init: %w", err)
	}
	return &Store{db: db}, nil
}

// tickRow is used only for parsing fields we need to store as columns.
type tickRow struct {
	Type   string  `json:"type"`
	Ts     int64   `json:"ts"`
	Price  float64 `json:"price"`
	Bid    float64 `json:"bid"`
	Ask    float64 `json:"ask"`
	Spread float64 `json:"spread"`
}

// StoreTick inserts one tick message. Non-tick messages are silently ignored.
func (s *Store) StoreTick(raw []byte) error {
	var t tickRow
	if err := json.Unmarshal(raw, &t); err != nil {
		return fmt.Errorf("parse tick: %w", err)
	}
	if t.Type != "tick" {
		return nil
	}

	_, err := s.db.Exec(
		`INSERT INTO ticks (ts, price, bid, ask, spread, raw) VALUES (?, ?, ?, ?, ?, ?)`,
		t.Ts, t.Price, t.Bid, t.Ask, t.Spread, string(raw),
	)
	if err != nil {
		return fmt.Errorf("insert tick: %w", err)
	}

	// Trim to the latest 10 000 ticks asynchronously so we never block
	// the event fan-out goroutine.
	go s.trimOldTicks()
	return nil
}

func (s *Store) trimOldTicks() {
	s.db.Exec(`DELETE FROM ticks WHERE id NOT IN (
		SELECT id FROM ticks ORDER BY id DESC LIMIT 10000
	)`)
}

// History returns up to limit ticks ordered oldest-first, as raw JSON.
func (s *Store) History(limit int) ([]json.RawMessage, error) {
	// Select the *most recent* limit rows, then order them oldest-first.
	rows, err := s.db.Query(`
		SELECT raw FROM (
			SELECT id, ts, raw FROM ticks ORDER BY id DESC LIMIT ?
		) ORDER BY ts ASC, id ASC
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("history query: %w", err)
	}
	defer rows.Close()

	var result []json.RawMessage
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		result = append(result, json.RawMessage(raw))
	}
	return result, rows.Err()
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
