package store

import "strings"

// Tag is a label that groups bookmarks.
type Tag struct {
	ID   int64
	Name string
}

// GetOrCreateTag returns the id of the tag matching displayName's normalized
// form, creating it (preserving displayName's casing) if absent.
func (s *Store) GetOrCreateTag(displayName string) (int64, error) {
	norm := NormalizeTag(displayName)
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM tags WHERE norm_name = ?`, norm).Scan(&id)
	if err == nil {
		return id, nil
	}
	res, err := s.DB.Exec(
		`INSERT INTO tags (name, norm_name) VALUES (?, ?)`,
		strings.TrimSpace(displayName), norm,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListTags returns all tags ordered by name.
func (s *Store) ListTags() ([]Tag, error) {
	rows, err := s.DB.Query(`SELECT id, name FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
