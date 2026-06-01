# inav Operations Layer + Management Endpoints Implementation Plan (Plan 2)

> **For agentic workers:** Implement task-by-task with TDD. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add the deterministic mutation operations (rename/merge tags, add/remove tags on bookmarks, retag, delete) and the HTTP management endpoints that expose them — the server side of the CRUD admin.

**Architecture:** Operations are implemented as `store` methods (the single mutation entry point — no raw SQL outside `store`). Each returns affected counts where meaningful, so a future v1.1 LLM "tidy assistant" can preview a plan built from the same operations. API handlers in `internal/api` call these methods; all are token-protected writes via the existing `RequireToken` middleware.

**Tech Stack:** same as Plan 1 (Go, SQLite, net/http). Builds on the merged backend-core.

**Conventions (from Plan 1, reused):** `store.Store`, `store.Tag`, `store.NewBookmark`, status constants, `NormalizeTag`, `EnqueueTagJob`, `api.Server`/`NewServer`/`NewRouter`, `RequireToken`. Go 1.22+ `net/http` path values (`r.PathValue("id")`).

**Vocabulary hygiene:** operations that can orphan a tag (remove-from-bookmark, delete-bookmark, merge) prune tags left with zero bookmarks, keeping the tag vocabulary clean (aligns with the spec's anti-explosion goal).

---

### Task 1: store — RenameTag (collision-aware)

`RenameTag(oldName, newName)` renames a tag's display + normalized key. If `newName` normalizes to the SAME tag (casing-only change), update display. If it collides with a DIFFERENT existing tag, merge the old tag into it. Otherwise rename in place.

**Files:** Create `internal/store/ops.go`; Test `internal/store/ops_test.go`

- [ ] **Step 1: Write the failing test**

`internal/store/ops_test.go`:
```go
package store

import "testing"

func TestRenameTagInPlace(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"golang"}, "")

	if err := s.RenameTag("golang", "Go"); err != nil {
		t.Fatal(err)
	}
	b, _ := s.GetBookmark(id)
	if len(b.Tags) != 1 || b.Tags[0] != "Go" {
		t.Fatalf("tags = %v, want [Go]", b.Tags)
	}
}

func TestRenameTagCasingOnly(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"go"}, "")

	if err := s.RenameTag("go", "Go"); err != nil {
		t.Fatal(err)
	}
	tags, _ := s.ListTags()
	if len(tags) != 1 || tags[0].Name != "Go" {
		t.Fatalf("tags = %+v, want one tag named Go", tags)
	}
}

func TestRenameTagMergesOnCollision(t *testing.T) {
	s := newTestStore(t)
	idA, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	idB, _ := s.UpsertBookmark(NewBookmark{URL: "https://b.com"})
	_ = s.SetBookmarkTags(idA, []string{"React"}, "")
	_ = s.SetBookmarkTags(idB, []string{"frontend"}, "")

	// Renaming React -> frontend collides; should merge into one tag.
	if err := s.RenameTag("React", "frontend"); err != nil {
		t.Fatal(err)
	}
	tags, _ := s.ListTags()
	if len(tags) != 1 {
		t.Fatalf("want 1 tag after merge, got %+v", tags)
	}
	a, _ := s.GetBookmark(idA)
	if len(a.Tags) != 1 || a.Tags[0] != "frontend" {
		t.Errorf("bookmark A tags = %v, want [frontend]", a.Tags)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/store/ -run TestRenameTag`
Expected: FAIL — `undefined: (*Store).RenameTag`.

- [ ] **Step 3: Implement** — create `internal/store/ops.go`:
```go
package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// mergeInto repoints all bookmark_tags from fromID to toID, then deletes the
// fromID tag. Caller supplies an open transaction.
func mergeInto(tx *sql.Tx, fromID, toID int64) error {
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO bookmark_tags (bookmark_id, tag_id)
		 SELECT bookmark_id, ? FROM bookmark_tags WHERE tag_id = ?`, toID, fromID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM bookmark_tags WHERE tag_id = ?`, fromID); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM tags WHERE id = ?`, fromID)
	return err
}

// RenameTag renames oldName to newName. Casing-only changes update the display
// name; a collision with a different existing tag merges into it; otherwise the
// tag is renamed in place.
func (s *Store) RenameTag(oldName, newName string) error {
	oldNorm := NormalizeTag(oldName)
	newNorm := NormalizeTag(newName)
	if newNorm == "" {
		return fmt.Errorf("new tag name is empty")
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldID int64
	if err := tx.QueryRow(`SELECT id FROM tags WHERE norm_name = ?`, oldNorm).Scan(&oldID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("tag %q not found", oldName)
		}
		return err
	}

	if oldNorm == newNorm {
		if _, err := tx.Exec(`UPDATE tags SET name = ? WHERE id = ?`, strings.TrimSpace(newName), oldID); err != nil {
			return err
		}
		return tx.Commit()
	}

	var otherID int64
	err = tx.QueryRow(`SELECT id FROM tags WHERE norm_name = ?`, newNorm).Scan(&otherID)
	switch {
	case err == sql.ErrNoRows:
		if _, err := tx.Exec(`UPDATE tags SET name = ?, norm_name = ? WHERE id = ?`,
			strings.TrimSpace(newName), newNorm, oldID); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if err := mergeInto(tx, oldID, otherID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
```

- [ ] **Step 4: Run, verify pass** — `go test ./internal/store/ -run TestRenameTag` → PASS
- [ ] **Step 5: Commit** — `git add internal/store/ops.go internal/store/ops_test.go && git commit -m "feat: RenameTag operation (collision-aware)"`

---

### Task 2: store — MergeTags

`MergeTags(sources []string, target string) (affected int, err)` merges every source tag into target (creating target if absent), returns the number of distinct bookmarks that ended up on target.

**Files:** Modify `internal/store/ops.go`; Test add to `internal/store/ops_test.go`

- [ ] **Step 1: Write the failing test** (append):
```go
func TestMergeTags(t *testing.T) {
	s := newTestStore(t)
	idA, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	idB, _ := s.UpsertBookmark(NewBookmark{URL: "https://b.com"})
	_ = s.SetBookmarkTags(idA, []string{"React"}, "")
	_ = s.SetBookmarkTags(idB, []string{"Vue", "React"}, "")

	affected, err := s.MergeTags([]string{"React", "Vue"}, "frontend")
	if err != nil {
		t.Fatal(err)
	}
	if affected != 2 {
		t.Errorf("affected = %d, want 2", affected)
	}
	tags, _ := s.ListTags()
	if len(tags) != 1 || tags[0].Name != "frontend" {
		t.Fatalf("want only [frontend], got %+v", tags)
	}
	a, _ := s.GetBookmark(idA)
	if len(a.Tags) != 1 || a.Tags[0] != "frontend" {
		t.Errorf("A tags = %v", a.Tags)
	}
}
```

- [ ] **Step 2: Run, verify fail** — `go test ./internal/store/ -run TestMergeTags` → FAIL
- [ ] **Step 3: Implement** (append to `internal/store/ops.go`):
```go
// MergeTags merges all source tags into target (created if absent) and returns
// the number of distinct bookmarks now carrying target.
func (s *Store) MergeTags(sources []string, target string) (int, error) {
	targetNorm := NormalizeTag(target)
	if targetNorm == "" {
		return 0, fmt.Errorf("target tag name is empty")
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var targetID int64
	err = tx.QueryRow(`SELECT id FROM tags WHERE norm_name = ?`, targetNorm).Scan(&targetID)
	if err == sql.ErrNoRows {
		res, e := tx.Exec(`INSERT INTO tags (name, norm_name) VALUES (?, ?)`, strings.TrimSpace(target), targetNorm)
		if e != nil {
			return 0, e
		}
		targetID, _ = res.LastInsertId()
	} else if err != nil {
		return 0, err
	}

	for _, src := range sources {
		srcNorm := NormalizeTag(src)
		if srcNorm == "" || srcNorm == targetNorm {
			continue
		}
		var srcID int64
		e := tx.QueryRow(`SELECT id FROM tags WHERE norm_name = ?`, srcNorm).Scan(&srcID)
		if e == sql.ErrNoRows {
			continue
		} else if e != nil {
			return 0, e
		}
		if err := mergeInto(tx, srcID, targetID); err != nil {
			return 0, err
		}
	}

	var affected int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM bookmark_tags WHERE tag_id = ?`, targetID).Scan(&affected); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}
```

- [ ] **Step 4: Run, verify pass** — `go test ./internal/store/ -run TestMergeTags` → PASS
- [ ] **Step 5: Commit** — `git commit -am "feat: MergeTags operation"`

---

### Task 3: store — Add/Remove tags on bookmarks (with orphan prune)

`AddTagToBookmarks(ids []int64, tag) (affected int, err)` and `RemoveTagFromBookmarks(ids []int64, tag) (affected int, err)`. Remove prunes the tag if it becomes orphaned.

**Files:** Modify `internal/store/ops.go`; Test append.

- [ ] **Step 1: Write the failing test** (append):
```go
func TestAddAndRemoveTag(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"Go"}, "")

	n, err := s.AddTagToBookmarks([]int64{id}, "web")
	if err != nil || n != 1 {
		t.Fatalf("add: n=%d err=%v", n, err)
	}
	b, _ := s.GetBookmark(id)
	if len(b.Tags) != 2 {
		t.Fatalf("tags = %v, want 2", b.Tags)
	}

	n, err = s.RemoveTagFromBookmarks([]int64{id}, "web")
	if err != nil || n != 1 {
		t.Fatalf("remove: n=%d err=%v", n, err)
	}
	b, _ = s.GetBookmark(id)
	if len(b.Tags) != 1 || b.Tags[0] != "Go" {
		t.Errorf("tags = %v, want [Go]", b.Tags)
	}
	// "web" had no other bookmarks → pruned.
	tags, _ := s.ListTags()
	for _, tg := range tags {
		if tg.Name == "web" {
			t.Error("orphan tag 'web' was not pruned")
		}
	}
}
```

- [ ] **Step 2: Run, verify fail**
- [ ] **Step 3: Implement** (append):
```go
// pruneOrphanTags deletes tags that no longer have any bookmarks.
func pruneOrphanTags(tx *sql.Tx) error {
	_, err := tx.Exec(`DELETE FROM tags WHERE id NOT IN (SELECT DISTINCT tag_id FROM bookmark_tags)`)
	return err
}

// AddTagToBookmarks attaches tag to each bookmark and returns how many rows were
// added (existing links are ignored).
func (s *Store) AddTagToBookmarks(ids []int64, tag string) (int, error) {
	norm := NormalizeTag(tag)
	if norm == "" {
		return 0, fmt.Errorf("tag name is empty")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var tagID int64
	err = tx.QueryRow(`SELECT id FROM tags WHERE norm_name = ?`, norm).Scan(&tagID)
	if err == sql.ErrNoRows {
		res, e := tx.Exec(`INSERT INTO tags (name, norm_name) VALUES (?, ?)`, strings.TrimSpace(tag), norm)
		if e != nil {
			return 0, e
		}
		tagID, _ = res.LastInsertId()
	} else if err != nil {
		return 0, err
	}

	affected := 0
	for _, id := range ids {
		res, e := tx.Exec(`INSERT OR IGNORE INTO bookmark_tags (bookmark_id, tag_id) VALUES (?, ?)`, id, tagID)
		if e != nil {
			return 0, e
		}
		if n, _ := res.RowsAffected(); n > 0 {
			affected++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

// RemoveTagFromBookmarks detaches tag from each bookmark, prunes the tag if it
// becomes orphaned, and returns how many links were removed.
func (s *Store) RemoveTagFromBookmarks(ids []int64, tag string) (int, error) {
	norm := NormalizeTag(tag)
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var tagID int64
	err = tx.QueryRow(`SELECT id FROM tags WHERE norm_name = ?`, norm).Scan(&tagID)
	if err == sql.ErrNoRows {
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	affected := 0
	for _, id := range ids {
		res, e := tx.Exec(`DELETE FROM bookmark_tags WHERE bookmark_id = ? AND tag_id = ?`, id, tagID)
		if e != nil {
			return 0, e
		}
		if n, _ := res.RowsAffected(); n > 0 {
			affected++
		}
	}
	if err := pruneOrphanTags(tx); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}
```

- [ ] **Step 4: Run, verify pass**
- [ ] **Step 5: Commit** — `git commit -am "feat: add/remove tag on bookmarks with orphan prune"`

---

### Task 4: store — RetagBookmark + DeleteBookmark

`RetagBookmark(id)` sets status back to pending and enqueues a fresh tag job. `DeleteBookmark(id)` deletes the bookmark (FK cascade clears its bookmark_tags and jobs) and prunes orphaned tags.

**Files:** Modify `internal/store/ops.go`; Test append.

- [ ] **Step 1: Write the failing test** (append):
```go
func TestRetagBookmark(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"Go"}, "sum") // now tagged

	if err := s.RetagBookmark(id); err != nil {
		t.Fatal(err)
	}
	b, _ := s.GetBookmark(id)
	if b.Status != StatusPending {
		t.Errorf("status = %q, want pending", b.Status)
	}
	if _, ok, _ := s.ClaimNextJob(); !ok {
		t.Error("expected a re-enqueued job")
	}
}

func TestDeleteBookmarkPrunesOrphanTags(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"solo"}, "")

	if err := s.DeleteBookmark(id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBookmark(id); err == nil {
		t.Error("expected bookmark to be gone")
	}
	tags, _ := s.ListTags()
	if len(tags) != 0 {
		t.Errorf("expected orphan tag pruned, got %+v", tags)
	}
}
```

- [ ] **Step 2: Run, verify fail**
- [ ] **Step 3: Implement** (append):
```go
// RetagBookmark resets a bookmark to pending and enqueues a fresh tagging job.
func (s *Store) RetagBookmark(id int64) error {
	if _, err := s.DB.Exec(`UPDATE bookmarks SET status = ? WHERE id = ?`, StatusPending, id); err != nil {
		return err
	}
	return s.EnqueueTagJob(id)
}

// DeleteBookmark removes a bookmark (its tag links and jobs cascade) and prunes
// any tags left orphaned.
func (s *Store) DeleteBookmark(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM bookmarks WHERE id = ?`, id); err != nil {
		return err
	}
	if err := pruneOrphanTags(tx); err != nil {
		return err
	}
	return tx.Commit()
}
```

- [ ] **Step 4: Run, verify pass** — `go test ./internal/store/` (all)
- [ ] **Step 5: Commit** — `git commit -am "feat: retag and delete bookmark operations"`

---

### Task 5: API — management endpoints + router wiring

Add handlers calling the new operations, and register routes. All are writes (token-protected by existing middleware).

Endpoints:
- `POST /api/tags/rename` — body `{"oldName":"","newName":""}` → `{"ok":true}`
- `POST /api/tags/merge` — body `{"sources":[],"target":""}` → `{"affected":N}`
- `PATCH /api/bookmarks/{id}/tags` — body `{"add":[],"remove":[]}` → updated bookmark tags `{"tags":[...]}`
- `POST /api/bookmarks/{id}/retag` → `{"id":N,"status":"pending"}`
- `DELETE /api/bookmarks/{id}` → 204

**Files:** Create `internal/api/manage.go`; Modify `internal/api/router.go`; Test `internal/api/manage_test.go`

- [ ] **Step 1: Write the failing test** — `internal/api/manage_test.go`:
```go
package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/52llm/iNav/internal/store"
)

func TestMergeTagsEndpoint(t *testing.T) {
	srv, s := newAPI(t)
	idA, _ := s.UpsertBookmark(store.NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(idA, []string{"React", "Vue"}, "")

	r := NewRouter(srv, "secret", false, emptyFS())
	req := httptest.NewRequest(http.MethodPost, "/api/tags/merge",
		strings.NewReader(`{"sources":["React","Vue"],"target":"frontend"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rr.Code, rr.Body)
	}
	tags, _ := s.ListTags()
	if len(tags) != 1 || tags[0].Name != "frontend" {
		t.Errorf("tags = %+v", tags)
	}
}

func TestDeleteBookmarkEndpoint(t *testing.T) {
	srv, s := newAPI(t)
	id, _ := s.UpsertBookmark(store.NewBookmark{URL: "https://a.com"})

	r := NewRouter(srv, "secret", false, emptyFS())
	req := httptest.NewRequest(http.MethodDelete, "/api/bookmarks/"+strconv.FormatInt(id, 10), nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("code = %d", rr.Code)
	}
	if _, err := s.GetBookmark(id); err == nil {
		t.Error("bookmark should be deleted")
	}
}

func TestPatchTagsEndpoint(t *testing.T) {
	srv, s := newAPI(t)
	id, _ := s.UpsertBookmark(store.NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"Go"}, "")

	r := NewRouter(srv, "secret", false, emptyFS())
	req := httptest.NewRequest(http.MethodPatch, "/api/bookmarks/"+strconv.FormatInt(id, 10)+"/tags",
		strings.NewReader(`{"add":["web"],"remove":["Go"]}`))
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rr.Code, rr.Body)
	}
	b, _ := s.GetBookmark(id)
	if len(b.Tags) != 1 || b.Tags[0] != "web" {
		t.Errorf("tags = %v, want [web]", b.Tags)
	}
}
```

Add this helper to `internal/api/manage_test.go` (a tiny empty static FS for the router):
```go
import "testing/fstest"

func emptyFS() fstest.MapFS {
	return fstest.MapFS{"index.html": {Data: []byte("x")}}
}
```

- [ ] **Step 2: Run, verify fail** — `go test ./internal/api/ -run 'TestMergeTagsEndpoint|TestDeleteBookmarkEndpoint|TestPatchTagsEndpoint'`
- [ ] **Step 3: Implement** — create `internal/api/manage.go`:
```go
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/52llm/iNav/internal/store"
)

func (srv *Server) RenameTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldName string `json:"oldName"`
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := srv.store.RenameTag(req.OldName, req.NewName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) MergeTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sources []string `json:"sources"`
		Target  string   `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	n, err := srv.store.MergeTags(req.Sources, req.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"affected": n})
}

func (srv *Server) PatchBookmarkTags(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	for _, tag := range req.Add {
		if _, err := srv.store.AddTagToBookmarks([]int64{id}, tag); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	for _, tag := range req.Remove {
		if _, err := srv.store.RemoveTagFromBookmarks([]int64{id}, tag); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	b, err := srv.store.GetBookmark(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	tags := b.Tags
	if tags == nil {
		tags = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (srv *Server) RetagBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := srv.store.RetagBookmark(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": store.StatusPending})
}

func (srv *Server) DeleteBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := srv.store.DeleteBookmark(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Register routes** — in `internal/api/router.go`, add to the `api` mux (after the existing handlers, before `protected := ...`):
```go
	api.HandleFunc("POST /api/tags/rename", srv.RenameTag)
	api.HandleFunc("POST /api/tags/merge", srv.MergeTags)
	api.HandleFunc("PATCH /api/bookmarks/{id}/tags", srv.PatchBookmarkTags)
	api.HandleFunc("POST /api/bookmarks/{id}/retag", srv.RetagBookmark)
	api.HandleFunc("DELETE /api/bookmarks/{id}", srv.DeleteBookmark)
```

- [ ] **Step 5: Run, verify pass** — `go test ./...` (all packages)
- [ ] **Step 6: Commit** — `git add internal/api/manage.go internal/api/manage_test.go internal/api/router.go && git commit -m "feat: management endpoints (rename/merge/patch-tags/retag/delete)"`

---

## Self-Review

**Spec coverage (design §5 operations layer, §3.4 CRUD management):**
- `rename_tag` → Task 1 ✓
- `merge_tags` → Task 2 ✓
- `add_tag` / `remove_tag` → Task 3 ✓
- `retag_bookmark` → Task 4 ✓
- `delete_bookmark` → Task 4 ✓
- `split_tag` → intentionally deferred (no v1 UI need; spec listed interface as optional). Noted, not built (YAGNI).
- Single mutation entry point (no raw SQL outside store) → all ops are store methods ✓
- Affected counts for future v1.1 preview → MergeTags/Add/Remove return counts ✓
- Vocabulary hygiene (orphan prune) → Task 3, 4 ✓

**Placeholder scan:** none.

**Type consistency:** `mergeInto`/`pruneOrphanTags` (private helpers, tx-scoped), `RenameTag`, `MergeTags`, `AddTagToBookmarks`, `RemoveTagFromBookmarks`, `RetagBookmark`, `DeleteBookmark` defined once; handlers reference the same signatures; router uses `r.PathValue("id")` consistent with `{id}` patterns.
