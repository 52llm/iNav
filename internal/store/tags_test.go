package store

import "testing"

func TestGetOrCreateTagDedupsByNorm(t *testing.T) {
	s := newTestStore(t)

	id1, err := s.GetOrCreateTag("React")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.GetOrCreateTag("  react ")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Errorf("expected same tag id for React/react, got %d and %d", id1, id2)
	}

	tags, err := s.ListTags()
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	// Display name preserves first-seen casing.
	if tags[0].Name != "React" {
		t.Errorf("display name = %q, want %q", tags[0].Name, "React")
	}
}
