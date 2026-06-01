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
