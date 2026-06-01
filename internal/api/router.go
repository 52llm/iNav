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
	api.HandleFunc("POST /api/tags/rename", srv.RenameTag)
	api.HandleFunc("POST /api/tags/merge", srv.MergeTags)
	api.HandleFunc("PATCH /api/bookmarks/{id}/tags", srv.PatchBookmarkTags)
	api.HandleFunc("POST /api/bookmarks/{id}/retag", srv.RetagBookmark)
	api.HandleFunc("DELETE /api/bookmarks/{id}", srv.DeleteBookmark)
	protected := RequireToken(token, publicRead, api)

	root := http.NewServeMux()
	root.Handle("/api/", protected)
	root.Handle("/", http.FileServer(http.FS(staticFS)))
	return root
}
