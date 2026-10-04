// Package store owns the SQLite file and every write the service performs.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// ErrConflict reports that a registration clashes with records already stored:
// either the namespace+locale pair exists with different values, or the region
// is already bound to another prefix.
var ErrConflict = errors.New("store: registration conflicts with existing records")

// MessageEntry is one registered copy text together with its routing fields.
type MessageEntry struct {
	Namespace string
	Locale    string
	Region    string
	Prefix    string
	Content   string
}

// Store wraps the SQLite handle so callers never touch database/sql directly.
type Store struct {
	db *sql.DB
	// mu serializes registration so the check-then-insert sequence inside
	// SaveMessageEntry cannot interleave with another registration.
	mu sync.Mutex
}

// Open prepares the database file and the schema this service needs.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable wal: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Ping reports whether the storage layer is usable.
func (s *Store) Ping() error { return s.db.Ping() }

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// SaveMessageEntry registers entry atomically. Re-submitting an identical entry
// returns the stored record with replayed set to true. Any disagreement with
// existing records yields ErrConflict and leaves storage untouched.
func (s *Store) SaveMessageEntry(entry MessageEntry) (MessageEntry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return MessageEntry{}, false, fmt.Errorf("begin registration: %w", err)
	}
	defer tx.Rollback()

	var existing MessageEntry
	err = tx.QueryRow(
		`SELECT namespace, locale, region, prefix, content FROM message_entries WHERE namespace = ? AND locale = ?`,
		entry.Namespace, entry.Locale,
	).Scan(&existing.Namespace, &existing.Locale, &existing.Region, &existing.Prefix, &existing.Content)
	switch {
	case err == nil:
		if existing != entry {
			return MessageEntry{}, false, ErrConflict
		}
		if err := tx.Commit(); err != nil {
			return MessageEntry{}, false, fmt.Errorf("commit replay: %w", err)
		}
		return existing, true, nil
	case !errors.Is(err, sql.ErrNoRows):
		return MessageEntry{}, false, fmt.Errorf("read entry: %w", err)
	}

	var boundPrefix string
	err = tx.QueryRow(`SELECT prefix FROM region_prefixes WHERE region = ?`, entry.Region).Scan(&boundPrefix)
	switch {
	case err == nil:
		if boundPrefix != entry.Prefix {
			return MessageEntry{}, false, ErrConflict
		}
	case !errors.Is(err, sql.ErrNoRows):
		return MessageEntry{}, false, fmt.Errorf("read region prefix: %w", err)
	default:
		if _, err := tx.Exec(
			`INSERT INTO region_prefixes (region, prefix) VALUES (?, ?)`,
			entry.Region, entry.Prefix,
		); err != nil {
			return MessageEntry{}, false, fmt.Errorf("insert region prefix: %w", err)
		}
	}

	if _, err := tx.Exec(
		`INSERT INTO message_entries (namespace, locale, region, prefix, content) VALUES (?, ?, ?, ?, ?)`,
		entry.Namespace, entry.Locale, entry.Region, entry.Prefix, entry.Content,
	); err != nil {
		return MessageEntry{}, false, fmt.Errorf("insert entry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return MessageEntry{}, false, fmt.Errorf("commit registration: %w", err)
	}
	return entry, false, nil
}

// FindMessageEntry loads the entry registered under namespace and locale.
// It returns sql.ErrNoRows when no such entry exists.
func (s *Store) FindMessageEntry(namespace, locale string) (MessageEntry, error) {
	var entry MessageEntry
	err := s.db.QueryRow(
		`SELECT namespace, locale, region, prefix, content FROM message_entries WHERE namespace = ? AND locale = ?`,
		namespace, locale,
	).Scan(&entry.Namespace, &entry.Locale, &entry.Region, &entry.Prefix, &entry.Content)
	if err != nil {
		return MessageEntry{}, err
	}
	return entry, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS service_metadata (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS message_entries (
	namespace TEXT NOT NULL,
	locale    TEXT NOT NULL,
	region    TEXT NOT NULL,
	prefix    TEXT NOT NULL,
	content   TEXT NOT NULL,
	PRIMARY KEY (namespace, locale)
);

CREATE TABLE IF NOT EXISTS region_prefixes (
	region TEXT PRIMARY KEY,
	prefix TEXT NOT NULL
);
`
