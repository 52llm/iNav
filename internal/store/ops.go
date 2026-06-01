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

// RetagBookmark resets a bookmark to pending and enqueues a fresh tagging job.
func (s *Store) RetagBookmark(id int64) error {
	if _, err := s.DB.Exec(`UPDATE bookmarks SET status = ? WHERE id = ?`, StatusPending, id); err != nil {
		return err
	}
	return s.EnqueueTagJob(id)
}

// ClearTagging removes all tags and tag links and clears every bookmark's
// summary, resetting bookmarks to pending. Bookmarks themselves (url, title,
// content) are kept. Use to start tagging from a clean slate, then RetagAll.
// Returns the number of bookmarks affected.
func (s *Store) ClearTagging() (int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM bookmark_tags`); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM tags`); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`UPDATE bookmarks SET summary = '', status = ?, tagged_at = NULL`, StatusPending)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(n), nil
}

// RetagAll resets every bookmark to pending and enqueues a fresh tagging job
// for each. Returns the number of bookmarks queued.
func (s *Store) RetagAll() (int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE bookmarks SET status = ?`, StatusPending); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`INSERT INTO jobs (bookmark_id, status) SELECT id, ? FROM bookmarks`, JobQueued)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(n), nil
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
