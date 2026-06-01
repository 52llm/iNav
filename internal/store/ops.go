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
