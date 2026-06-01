package store

import "strings"

// NormalizeTag produces the canonical matching key for a tag name:
// trims, collapses internal whitespace, and lowercases (a no-op for CJK).
// Two display names that normalize to the same key are treated as one tag.
func NormalizeTag(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ToLower(s)
}
