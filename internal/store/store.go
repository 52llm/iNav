package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Bookmark/job status constants.
const (
	StatusPending = "pending"
	StatusTagged  = "tagged"
	StatusFailed  = "failed"

	JobQueued  = "queued"
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
)

// Store wraps the SQLite database connection.
type Store struct {
	DB *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS bookmarks (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	url         TEXT NOT NULL UNIQUE,
	title       TEXT NOT NULL DEFAULT '',
	favicon_url TEXT NOT NULL DEFAULT '',
	excerpt     TEXT NOT NULL DEFAULT '',
	summary     TEXT NOT NULL DEFAULT '',
	content     TEXT NOT NULL DEFAULT '',
	status      TEXT NOT NULL DEFAULT 'pending',
	created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	tagged_at   DATETIME
);

CREATE TABLE IF NOT EXISTS tags (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	norm_name  TEXT NOT NULL UNIQUE,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS bookmark_tags (
	bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
	tag_id      INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
	PRIMARY KEY (bookmark_id, tag_id)
);

CREATE TABLE IF NOT EXISTS jobs (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
	status      TEXT NOT NULL DEFAULT 'queued',
	attempts    INTEGER NOT NULL DEFAULT 0,
	last_error  TEXT NOT NULL DEFAULT '',
	created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// Open opens (creating if needed) the SQLite database and runs migrations.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite handles concurrency with a single writer; cap connections to avoid
	// "database is locked" under the worker + API writing concurrently.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return &Store{DB: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.DB.Close() }
