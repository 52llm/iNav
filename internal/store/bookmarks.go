package store

import (
	"database/sql"
	"strings"
	"time"
)

// NewBookmark is the input for creating/updating a bookmark from a capture.
type NewBookmark struct {
	URL        string
	Title      string
	FaviconURL string
	Excerpt    string
	Content    string
}

// Bookmark is a stored bookmark with its tags populated on read.
type Bookmark struct {
	ID         int64
	URL        string
	Title      string
	FaviconURL string
	Excerpt    string
	Summary    string
	Content    string
	Status     string
	CreatedAt  time.Time
	TaggedAt   sql.NullTime
	Tags       []string
}

// BookmarkFilter narrows ListBookmarks results.
type BookmarkFilter struct {
	Tag string // normalized-matched tag name; empty = all
	Q   string // case-insensitive substring over title/url/summary
}

// UpsertBookmark inserts a bookmark or updates it by URL. On update, content
// fields are refreshed and status is reset to pending (so it gets re-tagged).
func (s *Store) UpsertBookmark(b NewBookmark) (int64, error) {
	_, err := s.DB.Exec(`
		INSERT INTO bookmarks (url, title, favicon_url, excerpt, content, status)
		VALUES (?, ?, ?, ?, ?, 'pending')
		ON CONFLICT(url) DO UPDATE SET
			title = excluded.title,
			favicon_url = excluded.favicon_url,
			excerpt = excluded.excerpt,
			content = excluded.content,
			status = 'pending'
	`, b.URL, b.Title, b.FaviconURL, b.Excerpt, b.Content)
	if err != nil {
		return 0, err
	}
	// LastInsertId is unreliable across the upsert path, so resolve by URL.
	var id int64
	err = s.DB.QueryRow(`SELECT id FROM bookmarks WHERE url = ?`, b.URL).Scan(&id)
	return id, err
}

// GetBookmark returns a single bookmark with its tags.
func (s *Store) GetBookmark(id int64) (Bookmark, error) {
	var b Bookmark
	err := s.DB.QueryRow(`
		SELECT id, url, title, favicon_url, excerpt, summary, content, status, created_at, tagged_at
		FROM bookmarks WHERE id = ?`, id).
		Scan(&b.ID, &b.URL, &b.Title, &b.FaviconURL, &b.Excerpt, &b.Summary, &b.Content, &b.Status, &b.CreatedAt, &b.TaggedAt)
	if err != nil {
		return Bookmark{}, err
	}
	b.Tags, err = s.tagsFor(b.ID)
	return b, err
}

func (s *Store) tagsFor(bookmarkID int64) ([]string, error) {
	rows, err := s.DB.Query(`
		SELECT t.name FROM tags t
		JOIN bookmark_tags bt ON bt.tag_id = t.id
		WHERE bt.bookmark_id = ? ORDER BY t.name`, bookmarkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// SetBookmarkTags replaces a bookmark's tags, sets its summary, and marks it
// tagged. Tag names are deduped via normalized matching.
func (s *Store) SetBookmarkTags(bookmarkID int64, tagNames []string, summary string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM bookmark_tags WHERE bookmark_id = ?`, bookmarkID); err != nil {
		return err
	}
	for _, name := range tagNames {
		norm := NormalizeTag(name)
		if norm == "" {
			continue
		}
		var tagID int64
		err := tx.QueryRow(`SELECT id FROM tags WHERE norm_name = ?`, norm).Scan(&tagID)
		if err == sql.ErrNoRows {
			res, err := tx.Exec(`INSERT INTO tags (name, norm_name) VALUES (?, ?)`, strings.TrimSpace(name), norm)
			if err != nil {
				return err
			}
			tagID, _ = res.LastInsertId()
		} else if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO bookmark_tags (bookmark_id, tag_id) VALUES (?, ?)`, bookmarkID, tagID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`UPDATE bookmarks SET summary = ?, status = ?, tagged_at = CURRENT_TIMESTAMP WHERE id = ?`,
		summary, StatusTagged, bookmarkID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ListBookmarks returns bookmarks (newest first) matching the filter, tags populated.
func (s *Store) ListBookmarks(f BookmarkFilter) ([]Bookmark, error) {
	query := `SELECT DISTINCT b.id, b.url, b.title, b.favicon_url, b.excerpt, b.summary, b.content, b.status, b.created_at, b.tagged_at
		FROM bookmarks b`
	var args []any
	var where []string
	if f.Tag != "" {
		query += ` JOIN bookmark_tags bt ON bt.bookmark_id = b.id JOIN tags t ON t.id = bt.tag_id`
		where = append(where, `t.norm_name = ?`)
		args = append(args, NormalizeTag(f.Tag))
	}
	if f.Q != "" {
		where = append(where, `(b.title LIKE ? OR b.url LIKE ? OR b.summary LIKE ?)`)
		like := "%" + f.Q + "%"
		args = append(args, like, like, like)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY b.created_at DESC, b.id DESC`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bookmark
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.ID, &b.URL, &b.Title, &b.FaviconURL, &b.Excerpt, &b.Summary, &b.Content, &b.Status, &b.CreatedAt, &b.TaggedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Tags, err = s.tagsFor(out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}
