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
