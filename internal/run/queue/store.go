// Package queue provides a client-side SQLite-backed work queue and session
// metadata store for the kinoko extraction pipeline.
package queue

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var ddl string

// Store manages the queue database (its own SQLite file, separate from the
// server index DB).
type Store struct {
	db *sql.DB
}

// New opens (or creates) the queue database at dsn, enables WAL mode, and
// applies the schema.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("queue: open %s: %w", dsn, err)
	}

	// Single writer settings for correctness.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("queue: %s: %w", p, err)
		}
	}

	if _, err := db.Exec(ddl); err != nil {
		db.Close()
		return nil, fmt.Errorf("queue: apply schema: %w", err)
	}

	return &Store{db: db}, nil
}

// DB returns the underlying *sql.DB for use by Queue and session helpers.
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }
