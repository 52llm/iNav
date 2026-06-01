package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/52llm/iNav/internal/store"
)

// RenameTag renames a tag (collision-aware in the store layer).
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

// MergeTags merges source tags into a target tag.
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

// PatchBookmarkTags adds and/or removes tags on a single bookmark.
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

// RetagBookmark re-enqueues a bookmark for tagging.
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

// ClearTagging wipes all tags/summaries and resets bookmarks to pending,
// keeping the bookmarks themselves. Follow with retag-all to regenerate.
func (srv *Server) ClearTagging(w http.ResponseWriter, r *http.Request) {
	n, err := srv.store.ClearTagging()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
}

// RetagAll re-enqueues every bookmark for tagging (e.g. after a prompt change).
func (srv *Server) RetagAll(w http.ResponseWriter, r *http.Request) {
	n, err := srv.store.RetagAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queued": n})
}

// DeleteBookmark removes a bookmark.
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
