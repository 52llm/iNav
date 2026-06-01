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
