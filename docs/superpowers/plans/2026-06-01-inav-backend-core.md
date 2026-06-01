# inav Backend Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the inav backend vertical slice — POST a captured page, store it as `pending`, asynchronously tag it with an OpenAI-compatible LLM, and read it back with tags + summary over HTTP.

**Architecture:** A single Go binary. `net/http` serves a JSON API; SQLite (`modernc.org/sqlite`, pure Go) is the only datastore; an in-process background worker polls a `jobs` table and calls the LLM. All data mutation in this plan goes through the store layer; the capture endpoint never blocks on the LLM.

**Tech Stack:** Go 1.22+ (`net/http` method+path routing, `database/sql`), `modernc.org/sqlite` (no CGo), standard `testing`. No external router, no Docker.

**Scope of this plan:** project scaffold, config, store (db + migrations, tags with normalization, bookmarks, jobs), LLM client (OpenAI-compatible), tagging worker, token auth middleware, bookmark create/list endpoints, and `main.go` wiring with embedded static placeholder. Tag/bookmark management operations (rename/merge/delete/retag) are **Plan 2**.

**Conventions used throughout:**
- Module path: `github.com/52llm/iNav`
- Status constants live in `internal/store/store.go`: `StatusPending = "pending"`, `StatusTagged = "tagged"`, `StatusFailed = "failed"`; job statuses `JobQueued = "queued"`, `JobRunning = "running"`, `JobDone = "done"`, `JobFailed = "failed"`.
- `maxAttempts = 3` for job retries.
- Tests open a fresh on-disk SQLite DB under `t.TempDir()` (avoids `:memory:` connection-pool sharing pitfalls).

---

### Task 1: Project scaffold + module

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `main.go` (temporary stub, replaced in Task 13)

- [ ] **Step 1: Initialize the Go module**

Run:
```bash
cd /Users/xiumu/git/me/inav
go mod init github.com/52llm/iNav
go get modernc.org/sqlite@latest
```
Expected: `go.mod` created with module path and a `require modernc.org/sqlite ...` line; `go.sum` populated.

- [ ] **Step 2: Add `.gitignore`**

Create `.gitignore`:
```
/inav
/inav.db
/inav.db-*
*.test
/web/dist/
```

- [ ] **Step 3: Add a temporary main stub so the module builds**

Create `main.go`:
```go
package main

import "fmt"

func main() {
	fmt.Println("inav: not wired yet")
}
```

- [ ] **Step 4: Verify the module builds**

Run: `go build ./...`
Expected: exits 0, no output.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum .gitignore main.go
git commit -m "chore: scaffold go module"
```

---

### Task 2: Config from environment

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:
```go
package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("INAV_TOKEN", "secret")
	t.Setenv("INAV_DB_PATH", "")
	t.Setenv("INAV_LISTEN_ADDR", "")
	t.Setenv("INAV_PUBLIC_READ", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Token != "secret" {
		t.Errorf("Token = %q, want %q", c.Token, "secret")
	}
	if c.DBPath != "inav.db" {
		t.Errorf("DBPath = %q, want %q", c.DBPath, "inav.db")
	}
	if c.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q", c.ListenAddr, ":8080")
	}
	if c.PublicRead {
		t.Errorf("PublicRead = true, want false")
	}
}

func TestLoadRequiresToken(t *testing.T) {
	t.Setenv("INAV_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when INAV_TOKEN is empty")
	}
}

func TestPublicReadTrue(t *testing.T) {
	t.Setenv("INAV_TOKEN", "x")
	t.Setenv("INAV_PUBLIC_READ", "true")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.PublicRead {
		t.Error("PublicRead = false, want true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/`
Expected: FAIL — `undefined: Load`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/config/config.go`:
```go
package config

import (
	"errors"
	"os"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	Token      string
	DBPath     string
	ListenAddr string
	PublicRead bool

	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string
}

// Load reads configuration from environment variables, applying defaults.
func Load() (Config, error) {
	c := Config{
		Token:      os.Getenv("INAV_TOKEN"),
		DBPath:     envOr("INAV_DB_PATH", "inav.db"),
		ListenAddr: envOr("INAV_LISTEN_ADDR", ":8080"),
		PublicRead: os.Getenv("INAV_PUBLIC_READ") == "true",
		LLMBaseURL: os.Getenv("INAV_LLM_BASE_URL"),
		LLMAPIKey:  os.Getenv("INAV_LLM_API_KEY"),
		LLMModel:   os.Getenv("INAV_LLM_MODEL"),
	}
	if c.Token == "" {
		return Config{}, errors.New("INAV_TOKEN is required")
	}
	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/`
Expected: PASS (`ok github.com/52llm/iNav/internal/config`).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: load config from environment"
```

---

### Task 3: Store — open DB + migrations

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/store/store_test.go`:
```go
package store

import (
	"path/filepath"
	"testing"
)

// newTestStore returns a Store backed by a fresh on-disk SQLite DB.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenRunsMigrations(t *testing.T) {
	s := newTestStore(t)
	// Querying a migrated table must succeed.
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM bookmarks`).Scan(&n); err != nil {
		t.Fatalf("bookmarks table not present: %v", err)
	}
	if n != 0 {
		t.Errorf("expected empty bookmarks, got %d", n)
	}
	for _, table := range []string{"tags", "bookmark_tags", "jobs"} {
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Errorf("table %q not present: %v", table, err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/`
Expected: FAIL — `undefined: Open`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/store/store.go`:
```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: open sqlite store with migrations"
```

---

### Task 4: Tag normalization (pure function)

**Files:**
- Create: `internal/store/normalize.go`
- Test: `internal/store/normalize_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/store/normalize_test.go`:
```go
package store

import "testing"

func TestNormalizeTag(t *testing.T) {
	cases := map[string]string{
		"  React ":   "react",
		"React":      "react",
		"front  end": "front end",
		"FRONTEND":   "frontend",
		"前端":         "前端",
		"":           "",
	}
	for in, want := range cases {
		if got := NormalizeTag(in); got != want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestNormalizeTag`
Expected: FAIL — `undefined: NormalizeTag`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/store/normalize.go`:
```go
package store

import "strings"

// NormalizeTag produces the canonical matching key for a tag name:
// trims, collapses internal whitespace, and lowercases (a no-op for CJK).
// Two display names that normalize to the same key are treated as one tag.
func NormalizeTag(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ToLower(s)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestNormalizeTag`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/normalize.go internal/store/normalize_test.go
git commit -m "feat: tag name normalization"
```

---

### Task 5: Store — tags (get-or-create with dedup)

**Files:**
- Create: `internal/store/tags.go`
- Test: `internal/store/tags_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/store/tags_test.go`:
```go
package store

import "testing"

func TestGetOrCreateTagDedupsByNorm(t *testing.T) {
	s := newTestStore(t)

	id1, err := s.GetOrCreateTag("React")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.GetOrCreateTag("  react ")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Errorf("expected same tag id for React/react, got %d and %d", id1, id2)
	}

	tags, err := s.ListTags()
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	// Display name preserves first-seen casing.
	if tags[0].Name != "React" {
		t.Errorf("display name = %q, want %q", tags[0].Name, "React")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestGetOrCreateTag`
Expected: FAIL — `undefined: (*Store).GetOrCreateTag` / `Tag`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/store/tags.go`:
```go
package store

import "strings"

// Tag is a label that groups bookmarks.
type Tag struct {
	ID   int64
	Name string
}

// GetOrCreateTag returns the id of the tag matching displayName's normalized
// form, creating it (preserving displayName's casing) if absent.
func (s *Store) GetOrCreateTag(displayName string) (int64, error) {
	norm := NormalizeTag(displayName)
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM tags WHERE norm_name = ?`, norm).Scan(&id)
	if err == nil {
		return id, nil
	}
	res, err := s.DB.Exec(
		`INSERT INTO tags (name, norm_name) VALUES (?, ?)`,
		strings.TrimSpace(displayName), norm,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListTags returns all tags ordered by name.
func (s *Store) ListTags() ([]Tag, error) {
	rows, err := s.DB.Query(`SELECT id, name FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestGetOrCreateTag`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/tags.go internal/store/tags_test.go
git commit -m "feat: get-or-create tags with dedup"
```

---

### Task 6: Store — bookmarks (upsert, get, list, set tags)

**Files:**
- Create: `internal/store/bookmarks.go`
- Test: `internal/store/bookmarks_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/store/bookmarks_test.go`:
```go
package store

import "testing"

func TestUpsertBookmarkIsIdempotentByURL(t *testing.T) {
	s := newTestStore(t)

	id1, err := s.UpsertBookmark(NewBookmark{
		URL: "https://example.com", Title: "Example", Content: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.UpsertBookmark(NewBookmark{
		URL: "https://example.com", Title: "Example Updated", Content: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("expected same id for same url, got %d and %d", id1, id2)
	}

	b, err := s.GetBookmark(id1)
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Example Updated" {
		t.Errorf("title = %q, want updated", b.Title)
	}
	if b.Status != StatusPending {
		t.Errorf("status = %q, want pending", b.Status)
	}
}

func TestSetBookmarkTagsAndList(t *testing.T) {
	s := newTestStore(t)
	id, err := s.UpsertBookmark(NewBookmark{URL: "https://a.com", Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBookmarkTags(id, []string{"Go", "web"}, "a one-line summary"); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListBookmarks(BookmarkFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(list))
	}
	got := list[0]
	if got.Status != StatusTagged {
		t.Errorf("status = %q, want tagged", got.Status)
	}
	if got.Summary != "a one-line summary" {
		t.Errorf("summary = %q", got.Summary)
	}
	if len(got.Tags) != 2 {
		t.Errorf("tags = %v, want 2", got.Tags)
	}
}

func TestListBookmarksFilterByTag(t *testing.T) {
	s := newTestStore(t)
	idA, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com", Title: "A"})
	idB, _ := s.UpsertBookmark(NewBookmark{URL: "https://b.com", Title: "B"})
	_ = s.SetBookmarkTags(idA, []string{"Go"}, "")
	_ = s.SetBookmarkTags(idB, []string{"Rust"}, "")

	list, err := s.ListBookmarks(BookmarkFilter{Tag: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].URL != "https://a.com" {
		t.Fatalf("tag filter failed: %+v", list)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestUpsertBookmark`
Expected: FAIL — `undefined: NewBookmark` / `UpsertBookmark`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/store/bookmarks.go`:
```go
package store

import (
	"database/sql"
	"strings"
	"time"
)

// NewBookmark is the input for creating/updating a bookmark from a capture.
type NewBookmark struct {
	URL        string
	Title      string
	FaviconURL string
	Excerpt    string
	Content    string
}

// Bookmark is a stored bookmark with its tags populated on read.
type Bookmark struct {
	ID         int64
	URL        string
	Title      string
	FaviconURL string
	Excerpt    string
	Summary    string
	Content    string
	Status     string
	CreatedAt  time.Time
	TaggedAt   sql.NullTime
	Tags       []string
}

// BookmarkFilter narrows ListBookmarks results.
type BookmarkFilter struct {
	Tag string // normalized-matched tag name; empty = all
	Q   string // case-insensitive substring over title/url/summary
}

// UpsertBookmark inserts a bookmark or updates it by URL. On update, content
// fields are refreshed and status is reset to pending (so it gets re-tagged).
func (s *Store) UpsertBookmark(b NewBookmark) (int64, error) {
	res, err := s.DB.Exec(`
		INSERT INTO bookmarks (url, title, favicon_url, excerpt, content, status)
		VALUES (?, ?, ?, ?, ?, 'pending')
		ON CONFLICT(url) DO UPDATE SET
			title = excluded.title,
			favicon_url = excluded.favicon_url,
			excerpt = excluded.excerpt,
			content = excluded.content,
			status = 'pending'
	`, b.URL, b.Title, b.FaviconURL, b.Excerpt, b.Content)
	if err != nil {
		return 0, err
	}
	if id, _ := res.LastInsertId(); id != 0 {
		// LastInsertId is only meaningful for a fresh insert; on conflict-update
		// it may be 0 or stale, so look up by URL to be safe.
		var realID int64
		if err := s.DB.QueryRow(`SELECT id FROM bookmarks WHERE url = ?`, b.URL).Scan(&realID); err != nil {
			return 0, err
		}
		return realID, nil
	}
	var id int64
	err = s.DB.QueryRow(`SELECT id FROM bookmarks WHERE url = ?`, b.URL).Scan(&id)
	return id, err
}

// GetBookmark returns a single bookmark with its tags.
func (s *Store) GetBookmark(id int64) (Bookmark, error) {
	var b Bookmark
	err := s.DB.QueryRow(`
		SELECT id, url, title, favicon_url, excerpt, summary, content, status, created_at, tagged_at
		FROM bookmarks WHERE id = ?`, id).
		Scan(&b.ID, &b.URL, &b.Title, &b.FaviconURL, &b.Excerpt, &b.Summary, &b.Content, &b.Status, &b.CreatedAt, &b.TaggedAt)
	if err != nil {
		return Bookmark{}, err
	}
	b.Tags, err = s.tagsFor(b.ID)
	return b, err
}

func (s *Store) tagsFor(bookmarkID int64) ([]string, error) {
	rows, err := s.DB.Query(`
		SELECT t.name FROM tags t
		JOIN bookmark_tags bt ON bt.tag_id = t.id
		WHERE bt.bookmark_id = ? ORDER BY t.name`, bookmarkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// SetBookmarkTags replaces a bookmark's tags, sets its summary, and marks it
// tagged. Tag names are deduped via GetOrCreateTag.
func (s *Store) SetBookmarkTags(bookmarkID int64, tagNames []string, summary string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM bookmark_tags WHERE bookmark_id = ?`, bookmarkID); err != nil {
		return err
	}
	for _, name := range tagNames {
		norm := NormalizeTag(name)
		if norm == "" {
			continue
		}
		var tagID int64
		err := tx.QueryRow(`SELECT id FROM tags WHERE norm_name = ?`, norm).Scan(&tagID)
		if err == sql.ErrNoRows {
			res, err := tx.Exec(`INSERT INTO tags (name, norm_name) VALUES (?, ?)`, strings.TrimSpace(name), norm)
			if err != nil {
				return err
			}
			tagID, _ = res.LastInsertId()
		} else if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO bookmark_tags (bookmark_id, tag_id) VALUES (?, ?)`, bookmarkID, tagID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`UPDATE bookmarks SET summary = ?, status = ?, tagged_at = CURRENT_TIMESTAMP WHERE id = ?`,
		summary, StatusTagged, bookmarkID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ListBookmarks returns bookmarks (newest first) matching the filter, tags populated.
func (s *Store) ListBookmarks(f BookmarkFilter) ([]Bookmark, error) {
	query := `SELECT DISTINCT b.id, b.url, b.title, b.favicon_url, b.excerpt, b.summary, b.content, b.status, b.created_at, b.tagged_at
		FROM bookmarks b`
	var args []any
	var where []string
	if f.Tag != "" {
		query += ` JOIN bookmark_tags bt ON bt.bookmark_id = b.id JOIN tags t ON t.id = bt.tag_id`
		where = append(where, `t.norm_name = ?`)
		args = append(args, NormalizeTag(f.Tag))
	}
	if f.Q != "" {
		where = append(where, `(b.title LIKE ? OR b.url LIKE ? OR b.summary LIKE ?)`)
		like := "%" + f.Q + "%"
		args = append(args, like, like, like)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY b.created_at DESC, b.id DESC`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bookmark
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.ID, &b.URL, &b.Title, &b.FaviconURL, &b.Excerpt, &b.Summary, &b.Content, &b.Status, &b.CreatedAt, &b.TaggedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Tags, err = s.tagsFor(out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run 'TestUpsertBookmark|TestSetBookmarkTags|TestListBookmarks'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/bookmarks.go internal/store/bookmarks_test.go
git commit -m "feat: bookmark upsert/get/list/set-tags"
```

---

### Task 7: Store — jobs (enqueue, claim, complete, fail)

**Files:**
- Create: `internal/store/jobs.go`
- Test: `internal/store/jobs_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/store/jobs_test.go`:
```go
package store

import "testing"

func TestEnqueueAndClaimJob(t *testing.T) {
	s := newTestStore(t)
	bID, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	if err := s.EnqueueTagJob(bID); err != nil {
		t.Fatal(err)
	}

	job, ok, err := s.ClaimNextJob()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a job to claim")
	}
	if job.BookmarkID != bID {
		t.Errorf("BookmarkID = %d, want %d", job.BookmarkID, bID)
	}
	if job.Status != JobRunning {
		t.Errorf("claimed job status = %q, want running", job.Status)
	}

	// A second claim finds nothing (the only job is now running).
	if _, ok, _ := s.ClaimNextJob(); ok {
		t.Error("expected no claimable job")
	}
}

func TestCompleteJob(t *testing.T) {
	s := newTestStore(t)
	bID, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.EnqueueTagJob(bID)
	job, _, _ := s.ClaimNextJob()

	if err := s.CompleteJob(job.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = s.DB.QueryRow(`SELECT status FROM jobs WHERE id = ?`, job.ID).Scan(&status)
	if status != JobDone {
		t.Errorf("job status = %q, want done", status)
	}
}

func TestFailJobRequeuesUntilMaxAttempts(t *testing.T) {
	s := newTestStore(t)
	bID, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.EnqueueTagJob(bID)

	// Fail it maxAttempts times; it should requeue until the cap, then mark failed.
	for i := 0; i < maxAttempts; i++ {
		job, ok, err := s.ClaimNextJob()
		if err != nil || !ok {
			t.Fatalf("attempt %d: claim failed ok=%v err=%v", i, ok, err)
		}
		if err := s.FailJob(job.ID, "boom"); err != nil {
			t.Fatal(err)
		}
	}
	// After maxAttempts failures there should be no claimable job left.
	if _, ok, _ := s.ClaimNextJob(); ok {
		t.Error("expected job to be terminally failed, but it was claimable")
	}
	var jobStatus, bmStatus string
	_ = s.DB.QueryRow(`SELECT status FROM jobs WHERE bookmark_id = ?`, bID).Scan(&jobStatus)
	_ = s.DB.QueryRow(`SELECT status FROM bookmarks WHERE id = ?`, bID).Scan(&bmStatus)
	if jobStatus != JobFailed {
		t.Errorf("job status = %q, want failed", jobStatus)
	}
	if bmStatus != StatusFailed {
		t.Errorf("bookmark status = %q, want failed", bmStatus)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestEnqueue|TestCompleteJob|TestFailJob'`
Expected: FAIL — `undefined: maxAttempts` / `EnqueueTagJob` / `ClaimNextJob` etc.

- [ ] **Step 3: Write minimal implementation**

Create `internal/store/jobs.go`:
```go
package store

import "database/sql"

const maxAttempts = 3

// Job is a queued tagging task for a bookmark.
type Job struct {
	ID         int64
	BookmarkID int64
	Status     string
	Attempts   int
}

// EnqueueTagJob adds a queued tagging job for the bookmark.
func (s *Store) EnqueueTagJob(bookmarkID int64) error {
	_, err := s.DB.Exec(
		`INSERT INTO jobs (bookmark_id, status) VALUES (?, ?)`,
		bookmarkID, JobQueued,
	)
	return err
}

// ClaimNextJob atomically marks the oldest queued job as running and returns it.
// ok is false when no job is available.
func (s *Store) ClaimNextJob() (Job, bool, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()

	var j Job
	err = tx.QueryRow(`
		SELECT id, bookmark_id, status, attempts FROM jobs
		WHERE status = ? ORDER BY id LIMIT 1`, JobQueued).
		Scan(&j.ID, &j.BookmarkID, &j.Status, &j.Attempts)
	if err == sql.ErrNoRows {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	if _, err := tx.Exec(
		`UPDATE jobs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		JobRunning, j.ID,
	); err != nil {
		return Job{}, false, err
	}
	j.Status = JobRunning
	return j, true, tx.Commit()
}

// CompleteJob marks a running job as done.
func (s *Store) CompleteJob(jobID int64) error {
	_, err := s.DB.Exec(
		`UPDATE jobs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		JobDone, jobID,
	)
	return err
}

// FailJob records an attempt failure. It requeues the job for retry until
// maxAttempts is reached, after which the job and its bookmark are marked failed.
func (s *Store) FailJob(jobID int64, errMsg string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var attempts, bookmarkID int64
	if err := tx.QueryRow(`SELECT attempts, bookmark_id FROM jobs WHERE id = ?`, jobID).
		Scan(&attempts, &bookmarkID); err != nil {
		return err
	}
	attempts++

	if attempts >= maxAttempts {
		if _, err := tx.Exec(
			`UPDATE jobs SET status = ?, attempts = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			JobFailed, attempts, errMsg, jobID,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE bookmarks SET status = ? WHERE id = ?`, StatusFailed, bookmarkID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(
			`UPDATE jobs SET status = ?, attempts = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			JobQueued, attempts, errMsg, jobID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/`
Expected: PASS (all store tests).

- [ ] **Step 5: Commit**

```bash
git add internal/store/jobs.go internal/store/jobs_test.go
git commit -m "feat: job queue enqueue/claim/complete/fail"
```

---

### Task 8: LLM client (OpenAI-compatible)

**Files:**
- Create: `internal/llm/llm.go`
- Test: `internal/llm/llm_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/llm/llm_test.go`:
```go
package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientTagParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/incorrect auth header: %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "existing-tag") {
			t.Errorf("existing tags not included in prompt: %s", body)
		}
		// Mimic an OpenAI chat completion whose message content is our JSON.
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{
					"content": `{"tags":["Go","web"],"summary":"A Go web framework."}`,
				}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key", "gpt-test")
	out, err := c.Tag(context.Background(), TagInput{
		Title:        "Gin",
		URL:          "https://gin-gonic.com",
		Content:      "Gin is a web framework written in Go.",
		ExistingTags: []string{"existing-tag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Summary != "A Go web framework." {
		t.Errorf("summary = %q", out.Summary)
	}
	if len(out.Tags) != 2 || out.Tags[0] != "Go" {
		t.Errorf("tags = %v", out.Tags)
	}
}

func TestClientTagErrorsOnMalformedContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "not json"}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "m")
	if _, err := c.Tag(context.Background(), TagInput{Title: "x"}); err == nil {
		t.Fatal("expected error on malformed JSON content")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/`
Expected: FAIL — `undefined: New` / `TagInput`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/llm/llm.go`:
```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TagInput is the page data the model uses to produce tags + a summary.
type TagInput struct {
	Title        string
	URL          string
	Excerpt      string
	Content      string
	ExistingTags []string
}

// TagResult is the model's structured output.
type TagResult struct {
	Tags    []string `json:"tags"`
	Summary string   `json:"summary"`
}

// Tagger produces tags + a summary for a captured page.
type Tagger interface {
	Tag(ctx context.Context, in TagInput) (TagResult, error)
}

// Client calls an OpenAI-compatible /chat/completions endpoint.
type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// New builds a Client. baseURL is e.g. "https://api.openai.com/v1".
func New(baseURL, apiKey, model string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Tag(ctx context.Context, in TagInput) (TagResult, error) {
	content := strings.TrimSpace(in.Content)
	if len(content) > 6000 {
		content = content[:6000]
	}
	prompt := fmt.Sprintf(`You categorize web pages for a personal bookmark manager.
Return ONLY a JSON object: {"tags": ["..."], "summary": "one concise sentence"}.
Rules:
- Prefer reusing an EXISTING tag when one fits; only invent a new tag if none apply.
- Use 1-4 tags. Keep tag names short and consistent in language with the page.
- summary: one neutral sentence describing what the page is.

EXISTING TAGS: %s

PAGE TITLE: %s
PAGE URL: %s
PAGE EXCERPT: %s
PAGE CONTENT:
%s`,
		strings.Join(in.ExistingTags, ", "), in.Title, in.URL, in.Excerpt, content)

	reqBody := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0,
	}
	buf, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return TagResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return TagResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return TagResult{}, fmt.Errorf("llm http %d", resp.StatusCode)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return TagResult{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return TagResult{}, fmt.Errorf("llm returned no choices")
	}

	var out TagResult
	if err := json.Unmarshal([]byte(parsed.Choices[0].Message.Content), &out); err != nil {
		return TagResult{}, fmt.Errorf("parse model content as json: %w", err)
	}
	if len(out.Tags) == 0 {
		return TagResult{}, fmt.Errorf("model returned no tags")
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/llm/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/llm/
git commit -m "feat: openai-compatible llm tagging client"
```

---

### Task 9: Tagging worker

**Files:**
- Create: `internal/tagger/tagger.go`
- Test: `internal/tagger/tagger_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/tagger/tagger_test.go`:
```go
package tagger

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/52llm/iNav/internal/llm"
	"github.com/52llm/iNav/internal/store"
)

type fakeTagger struct {
	result llm.TagResult
	err    error
	seen   []string // ExistingTags passed in on the last call
}

func (f *fakeTagger) Tag(_ context.Context, in llm.TagInput) (llm.TagResult, error) {
	f.seen = in.ExistingTags
	return f.result, f.err
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProcessOneTagsBookmark(t *testing.T) {
	s := newStore(t)
	bID, _ := s.UpsertBookmark(store.NewBookmark{URL: "https://a.com", Title: "A"})
	_ = s.EnqueueTagJob(bID)

	w := New(s, &fakeTagger{result: llm.TagResult{Tags: []string{"Go"}, Summary: "sum"}})
	did, err := w.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !did {
		t.Fatal("expected ProcessOne to do work")
	}

	b, _ := s.GetBookmark(bID)
	if b.Status != store.StatusTagged {
		t.Errorf("status = %q, want tagged", b.Status)
	}
	if b.Summary != "sum" || len(b.Tags) != 1 {
		t.Errorf("tags/summary not applied: %+v", b)
	}
}

func TestProcessOneNoWork(t *testing.T) {
	s := newStore(t)
	w := New(s, &fakeTagger{})
	did, err := w.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if did {
		t.Error("expected no work with empty queue")
	}
}

func TestProcessOneFailsJobOnLLMError(t *testing.T) {
	s := newStore(t)
	bID, _ := s.UpsertBookmark(store.NewBookmark{URL: "https://a.com"})
	_ = s.EnqueueTagJob(bID)

	w := New(s, &fakeTagger{err: errors.New("llm down")})
	if _, err := w.ProcessOne(context.Background()); err != nil {
		t.Fatalf("ProcessOne should swallow llm errors, got %v", err)
	}
	// Job requeued (attempts < max), bookmark still pending.
	b, _ := s.GetBookmark(bID)
	if b.Status != store.StatusPending {
		t.Errorf("status = %q, want pending after first failure", b.Status)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tagger/`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/tagger/tagger.go`:
```go
package tagger

import (
	"context"
	"log"
	"time"

	"github.com/52llm/iNav/internal/llm"
	"github.com/52llm/iNav/internal/store"
)

// Worker polls the job queue and tags bookmarks via an llm.Tagger.
type Worker struct {
	store  *store.Store
	tagger llm.Tagger
}

// New builds a Worker.
func New(s *store.Store, t llm.Tagger) *Worker {
	return &Worker{store: s, tagger: t}
}

// ProcessOne claims at most one job and processes it. It reports whether work
// was done. LLM/processing failures are recorded against the job (and the job
// requeued or failed), not returned as errors.
func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	job, ok, err := w.store.ClaimNextJob()
	if err != nil || !ok {
		return false, err
	}

	b, err := w.store.GetBookmark(job.BookmarkID)
	if err != nil {
		_ = w.store.FailJob(job.ID, "load bookmark: "+err.Error())
		return true, nil
	}

	existing, err := w.store.ListTags()
	if err != nil {
		_ = w.store.FailJob(job.ID, "list tags: "+err.Error())
		return true, nil
	}
	names := make([]string, len(existing))
	for i, t := range existing {
		names[i] = t.Name
	}

	res, err := w.tagger.Tag(ctx, llm.TagInput{
		Title:        b.Title,
		URL:          b.URL,
		Excerpt:      b.Excerpt,
		Content:      b.Content,
		ExistingTags: names,
	})
	if err != nil {
		_ = w.store.FailJob(job.ID, err.Error())
		return true, nil
	}

	if err := w.store.SetBookmarkTags(job.BookmarkID, res.Tags, res.Summary); err != nil {
		_ = w.store.FailJob(job.ID, "set tags: "+err.Error())
		return true, nil
	}
	return true, w.store.CompleteJob(job.ID)
}

// Run loops ProcessOne until ctx is cancelled, sleeping when the queue is empty.
func (w *Worker) Run(ctx context.Context) {
	const idle = 2 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		did, err := w.ProcessOne(ctx)
		if err != nil {
			log.Printf("tagger: %v", err)
		}
		if !did {
			select {
			case <-ctx.Done():
				return
			case <-time.After(idle):
			}
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tagger/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tagger/
git commit -m "feat: background tagging worker"
```

---

### Task 10: Auth middleware

**Files:**
- Create: `internal/api/auth.go`
- Test: `internal/api/auth_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/auth_test.go`:
```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
}

func TestRequireTokenRejectsMissing(t *testing.T) {
	h := RequireToken("secret", false, okHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/bookmarks", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rr.Code)
	}
}

func TestRequireTokenAcceptsValid(t *testing.T) {
	h := RequireToken("secret", false, okHandler())
	req := httptest.NewRequest(http.MethodPost, "/api/bookmarks", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", rr.Code)
	}
}

func TestRequireTokenPublicReadAllowsGET(t *testing.T) {
	h := RequireToken("secret", true, okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET with publicRead should pass, code = %d", rr.Code)
	}
	// A write still needs the token even with publicRead.
	req2 := httptest.NewRequest(http.MethodPost, "/api/bookmarks", nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("POST with publicRead must still 401, code = %d", rr2.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestRequireToken`
Expected: FAIL — `undefined: RequireToken`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/api/auth.go`:
```go
package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireToken wraps next, enforcing a Bearer token. When publicRead is true,
// safe (GET/HEAD) requests are allowed without a token; writes always need it.
func RequireToken(token string, publicRead bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicRead && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestRequireToken`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go
git commit -m "feat: bearer token auth middleware"
```

---

### Task 11: API — bookmark create + list handlers

**Files:**
- Create: `internal/api/handlers.go`
- Test: `internal/api/handlers_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/handlers_test.go`:
```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/52llm/iNav/internal/store"
)

func newAPI(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewServer(s), s
}

func TestCreateBookmarkEnqueuesJob(t *testing.T) {
	srv, s := newAPI(t)
	body := `{"url":"https://a.com","title":"A","content":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/api/bookmarks", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.CreateBookmark(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d body = %s", rr.Code, rr.Body)
	}
	var resp struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != store.StatusPending {
		t.Errorf("status = %q, want pending", resp.Status)
	}
	// A job must have been enqueued.
	if _, ok, _ := s.ClaimNextJob(); !ok {
		t.Error("expected an enqueued job")
	}
}

func TestCreateBookmarkRejectsBadURL(t *testing.T) {
	srv, _ := newAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/bookmarks", strings.NewReader(`{"title":"no url"}`))
	rr := httptest.NewRecorder()
	srv.CreateBookmark(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rr.Code)
	}
}

func TestListBookmarks(t *testing.T) {
	srv, s := newAPI(t)
	id, _ := s.UpsertBookmark(store.NewBookmark{URL: "https://a.com", Title: "A"})
	_ = s.SetBookmarkTags(id, []string{"Go"}, "sum")

	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	rr := httptest.NewRecorder()
	srv.ListBookmarks(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d", rr.Code)
	}
	var resp []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 1 {
		t.Fatalf("want 1 bookmark, got %d", len(resp))
	}
	if resp[0]["summary"] != "sum" {
		t.Errorf("summary = %v", resp[0]["summary"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run 'TestCreateBookmark|TestListBookmarks'`
Expected: FAIL — `undefined: NewServer` / `Server`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/api/handlers.go`:
```go
package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/52llm/iNav/internal/store"
)

// Server holds API dependencies.
type Server struct {
	store *store.Store
}

// NewServer builds a Server.
func NewServer(s *store.Store) *Server { return &Server{store: s} }

type createBookmarkRequest struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	FaviconURL string `json:"faviconUrl"`
	Excerpt    string `json:"excerpt"`
	Content    string `json:"content"`
}

type bookmarkResponse struct {
	ID         int64    `json:"id"`
	URL        string   `json:"url"`
	Title      string   `json:"title"`
	FaviconURL string   `json:"faviconUrl"`
	Summary    string   `json:"summary"`
	Status     string   `json:"status"`
	Tags       []string `json:"tags"`
}

// CreateBookmark upserts a captured page and enqueues it for tagging.
func (srv *Server) CreateBookmark(w http.ResponseWriter, r *http.Request) {
	var req createBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if u, err := url.ParseRequestURI(req.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		http.Error(w, "valid http(s) url required", http.StatusBadRequest)
		return
	}

	id, err := srv.store.UpsertBookmark(store.NewBookmark{
		URL: req.URL, Title: req.Title, FaviconURL: req.FaviconURL,
		Excerpt: req.Excerpt, Content: req.Content,
	})
	if err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	if err := srv.store.EnqueueTagJob(id); err != nil {
		http.Error(w, "enqueue error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": store.StatusPending})
}

// ListBookmarks returns bookmarks, optionally filtered by ?tag= and ?q=.
func (srv *Server) ListBookmarks(w http.ResponseWriter, r *http.Request) {
	list, err := srv.store.ListBookmarks(store.BookmarkFilter{
		Tag: r.URL.Query().Get("tag"),
		Q:   r.URL.Query().Get("q"),
	})
	if err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	out := make([]bookmarkResponse, 0, len(list))
	for _, b := range list {
		tags := b.Tags
		if tags == nil {
			tags = []string{}
		}
		out = append(out, bookmarkResponse{
			ID: b.ID, URL: b.URL, Title: b.Title, FaviconURL: b.FaviconURL,
			Summary: b.Summary, Status: b.Status, Tags: tags,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// ListTags returns all known tag names.
func (srv *Server) ListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := srv.store.ListTags()
	if err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	writeJSON(w, http.StatusOK, names)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/`
Expected: PASS (handlers + auth).

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers.go internal/api/handlers_test.go
git commit -m "feat: bookmark create/list/tags handlers"
```

---

### Task 12: Router (wires handlers + auth + static)

**Files:**
- Create: `internal/api/router.go`
- Test: `internal/api/router_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/router_test.go`:
```go
package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/52llm/iNav/internal/store"
)

func TestRouterRoutesAndAuth(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>nav</html>")}}
	var sub fs.FS = staticFS

	h := NewRouter(NewServer(s), "secret", false, sub)
	ts := httptest.NewServer(h)
	defer ts.Close()

	// Unauthenticated write -> 401.
	resp, _ := http.Post(ts.URL+"/api/bookmarks", "application/json", strings.NewReader(`{"url":"https://a.com"}`))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauth POST code = %d, want 401", resp.StatusCode)
	}

	// Authenticated write -> 200.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/bookmarks", strings.NewReader(`{"url":"https://a.com"}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req)
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("auth POST code = %d, want 200", resp2.StatusCode)
	}

	// Static index served at root.
	resp3, _ := http.Get(ts.URL + "/")
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("static index code = %d, want 200", resp3.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestRouterRoutes`
Expected: FAIL — `undefined: NewRouter`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/api/router.go`:
```go
package api

import (
	"io/fs"
	"net/http"
)

// NewRouter wires API endpoints (token-protected) and serves the static nav
// site from staticFS at the root.
func NewRouter(srv *Server, token string, publicRead bool, staticFS fs.FS) http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("POST /api/bookmarks", srv.CreateBookmark)
	api.HandleFunc("GET /api/bookmarks", srv.ListBookmarks)
	api.HandleFunc("GET /api/tags", srv.ListTags)
	protected := RequireToken(token, publicRead, api)

	root := http.NewServeMux()
	root.Handle("/api/", protected)
	root.Handle("/", http.FileServer(http.FS(staticFS)))
	return root
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/router.go internal/api/router_test.go
git commit -m "feat: http router wiring api, auth, static"
```

---

### Task 13: Embed static + wire `main.go`

**Files:**
- Create: `internal/web/web.go`
- Create: `web/dist/index.html` (placeholder until Plan 4 builds the real nav site)
- Modify: `main.go` (replace the Task 1 stub)
- Test: `internal/web/web_test.go`

- [ ] **Step 1: Create the placeholder static asset**

Create `web/dist/index.html`:
```html
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>inav</title></head>
<body><h1>inav backend is running</h1><p>The nav site UI ships in a later plan.</p></body>
</html>
```

- [ ] **Step 2: Write the failing test for the embedded FS**

Create `internal/web/web_test.go`:
```go
package web

import (
	"io/fs"
	"testing"
)

func TestDistHasIndex(t *testing.T) {
	f, err := Dist()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(f, "index.html"); err != nil {
		t.Errorf("index.html missing from embedded dist: %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/web/`
Expected: FAIL — `undefined: Dist`.

- [ ] **Step 4: Write the embed implementation**

Create `internal/web/web.go`:
```go
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the embedded nav-site static files rooted at the dist directory.
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
```

Note: `go:embed` paths are relative to the source file, so move/copy the placeholder to `internal/web/dist/index.html`:
```bash
mkdir -p internal/web/dist
cp web/dist/index.html internal/web/dist/index.html
```
(Plan 4's build step will output the real nav site into `internal/web/dist/`.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/web/`
Expected: PASS.

- [ ] **Step 6: Replace `main.go` with real wiring**

Replace `main.go` entirely with:
```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/52llm/iNav/internal/api"
	"github.com/52llm/iNav/internal/config"
	"github.com/52llm/iNav/internal/llm"
	"github.com/52llm/iNav/internal/store"
	"github.com/52llm/iNav/internal/tagger"
	"github.com/52llm/iNav/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	staticFS, err := web.Dist()
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Background tagging worker.
	llmClient := llm.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	worker := tagger.New(st, llmClient)
	go worker.Run(ctx)

	handler := api.NewRouter(api.NewServer(st), cfg.Token, cfg.PublicRead, staticFS)
	httpServer := &http.Server{Addr: cfg.ListenAddr, Handler: handler}

	go func() {
		log.Printf("inav listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	log.Println("inav stopped")
	_ = os.Stdout.Sync()
}
```

- [ ] **Step 7: Build and run the full test suite**

Run:
```bash
go build ./...
go test ./...
```
Expected: build succeeds; all packages PASS.

- [ ] **Step 8: Manual smoke test**

Run:
```bash
INAV_TOKEN=dev INAV_DB_PATH=./smoke.db ./... # build first:
go build -o inav . && INAV_TOKEN=dev INAV_DB_PATH=./smoke.db ./inav &
sleep 1
curl -s -XPOST localhost:8080/api/bookmarks -H 'Authorization: Bearer dev' \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://gin-gonic.com","title":"Gin","content":"Gin is a Go web framework"}'
curl -s localhost:8080/api/bookmarks -H 'Authorization: Bearer dev'
```
Expected: first curl returns `{"id":1,"status":"pending"}`; second returns a JSON array containing the bookmark (status `pending` — it stays pending without a real LLM endpoint configured, which is correct). Stop the server with `kill %1` and remove `smoke.db`.

- [ ] **Step 9: Commit**

```bash
git add internal/web/ web/ main.go .gitignore
git commit -m "feat: embed static and wire main entrypoint"
```

---

## Self-Review

**Spec coverage (against `2026-06-01-inav-design.md`):**
- §2 architecture (Go binary, SQLite, embedded static, in-process worker) → Tasks 3, 9, 13 ✓
- §3.1 capture (upsert by URL, pending, enqueue, fast return) → Tasks 6, 7, 11 ✓
- §3.2 async tagging (worker, existing-tag reuse, summary, retry/fail) → Tasks 8, 9 ✓
- §3.3 browse (list, filter by tag, search) → Tasks 6, 11 ✓
- §4 data model (bookmarks/tags/bookmark_tags/jobs) → Task 3 ✓ (refinement: `tags` has `name` + unique `norm_name` instead of unique `name`, to preserve display casing while deduping — consistent with §6 防爆炸 intent)
- §6 tag anti-explosion (reuse-first prompt + normalization) → Tasks 4, 5, 8 ✓
- §7 LLM (OpenAI-compatible, config-driven, JSON validation) → Tasks 2, 8 ✓
- §8 auth (token, optional public read) → Tasks 2, 10 ✓
- §9 config (env vars) → Task 2 ✓
- §10 error handling (capture never blocks; worker retries; failed surfaced) → Tasks 9, 11 ✓
- §5 operations layer + §3.4 CRUD management → **deferred to Plan 2** (noted; not a gap in this plan's scope)

**Placeholder scan:** No TBD/TODO; every code step contains complete code.

**Type consistency:** `NewBookmark`, `Bookmark`, `Tag`, `Job`, `BookmarkFilter`, `llm.TagInput`, `llm.TagResult`, `llm.Tagger`, `store.Status*`/`store.Job*` constants, `Server`, `NewServer`, `NewRouter`, `RequireToken`, `Worker.New/ProcessOne/Run`, `web.Dist` are defined once and referenced consistently across tasks.
