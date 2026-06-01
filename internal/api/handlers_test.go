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
