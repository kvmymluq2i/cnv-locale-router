// Package store owns the SQLite file and every write the service performs.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite handle so callers never touch database/sql directly.
type Store struct {
	db *sql.DB
	// writeMu serializes registrations so the conflict checks and the inserts
	// stay atomic relative to one another.
	writeMu sync.Mutex
}

// Entry is one registered message together with the prefix currently mapped
// to its region. All fields carry already-validated, normalized values.
type Entry struct {
	Namespace string
	Locale    string
	Region    string
	Prefix    string
	Content   string
}

// ConflictError signals that a registration collides with stored state: either
// the (namespace, locale) pair already exists with different values, or the
// region is already mapped to a different prefix.
type ConflictError struct{}

func (ConflictError) Error() string { return "message entry conflicts with an existing registration" }

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
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	// A single connection serializes access and keeps WAL writes from racing
	// one another ("database is locked").
	db.SetMaxOpenConns(1)
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

// PutEntry registers a message entry and, when first seen, its region-to-prefix
// mapping, all in one transaction. An identical repeat (same region, prefix and
// content) succeeds without writing anything. Any mismatch with an existing
// registration — the (namespace, locale) row differs, or an existing region is
// submitted with a different prefix — returns ConflictError and leaves stored
// state untouched.
func (s *Store) PutEntry(entry Entry) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	var existingRegion, existingContent string
	err = tx.QueryRow(
		`SELECT region, content FROM message_entries WHERE namespace = ? AND locale = ?`,
		entry.Namespace, entry.Locale,
	).Scan(&existingRegion, &existingContent)
	switch {
	case err == nil:
		// The prefix follows the region through region_prefixes, so matching
		// region and content implies an identical repeat.
		if existingRegion != entry.Region || existingContent != entry.Content {
			return ConflictError{}
		}
		var mappedPrefix string
		if err := tx.QueryRow(
			`SELECT prefix FROM region_prefixes WHERE region = ?`,
			entry.Region,
		).Scan(&mappedPrefix); err != nil {
			return fmt.Errorf("load region mapping: %w", err)
		}
		if mappedPrefix != entry.Prefix {
			return ConflictError{}
		}
		return nil // exact repeat: nothing to write
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("load entry: %w", err)
	}

	// The (namespace, locale) row is free. The region may already be mapped by
	// another entry: the same prefix is fine, a different one is a conflict.
	var mappedPrefix string
	switch err := tx.QueryRow(
		`SELECT prefix FROM region_prefixes WHERE region = ?`,
		entry.Region,
	).Scan(&mappedPrefix); {
	case err == nil:
		if mappedPrefix != entry.Prefix {
			return ConflictError{}
		}
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(
			`INSERT INTO region_prefixes (region, prefix) VALUES (?, ?)`,
			entry.Region, entry.Prefix,
		); err != nil {
			return fmt.Errorf("insert region mapping: %w", err)
		}
	default:
		return fmt.Errorf("load region mapping: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO message_entries (namespace, locale, region, content) VALUES (?, ?, ?, ?)`,
		entry.Namespace, entry.Locale, entry.Region, entry.Content,
	); err != nil {
		return fmt.Errorf("insert entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Route resolves the registered entry for a namespace and normalized locale:
// the entry names a region and the prefix is read from the region mapping
// table. No language fallback is attempted.
func (s *Store) Route(namespace, normalizedLocale string) (Entry, bool, error) {
	var entry Entry
	err := s.db.QueryRow(
		`SELECT m.namespace, m.locale, m.region, r.prefix, m.content
		 FROM message_entries AS m
		 JOIN region_prefixes AS r ON r.region = m.region
		 WHERE m.namespace = ? AND m.locale = ?`,
		namespace, normalizedLocale,
	).Scan(&entry.Namespace, &entry.Locale, &entry.Region, &entry.Prefix, &entry.Content)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("load route: %w", err)
	}
	return entry, true, nil
}

// PrefixForRegion returns the prefix currently mapped to a region.
func (s *Store) PrefixForRegion(region string) (string, bool, error) {
	var prefix string
	err := s.db.QueryRow(`SELECT prefix FROM region_prefixes WHERE region = ?`, region).Scan(&prefix)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load prefix: %w", err)
	}
	return prefix, true, nil
}

// IsConflict reports whether err came from a registration conflict.
func IsConflict(err error) bool {
	var conflict ConflictError
	return errors.As(err, &conflict)
}

const schema = `
CREATE TABLE IF NOT EXISTS service_metadata (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS region_prefixes (
	region TEXT PRIMARY KEY,
	prefix TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS message_entries (
	namespace TEXT NOT NULL,
	locale    TEXT NOT NULL,
	region    TEXT NOT NULL,
	content   TEXT NOT NULL,
	PRIMARY KEY (namespace, locale),
	FOREIGN KEY (region) REFERENCES region_prefixes(region)
);
`
