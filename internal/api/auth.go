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
