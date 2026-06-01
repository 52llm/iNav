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
