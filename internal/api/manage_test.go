package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/52llm/iNav/internal/store"
)

func emptyFS() fstest.MapFS {
	return fstest.MapFS{"index.html": {Data: []byte("x")}}
}

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

func TestRetagAllEndpoint(t *testing.T) {
	srv, s := newAPI(t)
	a, _ := s.UpsertBookmark(store.NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(a, []string{"x"}, "s")

	r := NewRouter(srv, "secret", false, emptyFS())
	req := httptest.NewRequest(http.MethodPost, "/api/bookmarks/retag-all", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rr.Code, rr.Body)
	}
	b, _ := s.GetBookmark(a)
	if b.Status != store.StatusPending {
		t.Errorf("status = %q, want pending after retag-all", b.Status)
	}
	if _, ok, _ := s.ClaimNextJob(); !ok {
		t.Error("expected a queued job after retag-all")
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
