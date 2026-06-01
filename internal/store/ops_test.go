package store

import "testing"

func TestRenameTagInPlace(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"golang"}, "")

	if err := s.RenameTag("golang", "Go"); err != nil {
		t.Fatal(err)
	}
	b, _ := s.GetBookmark(id)
	if len(b.Tags) != 1 || b.Tags[0] != "Go" {
		t.Fatalf("tags = %v, want [Go]", b.Tags)
	}
}

func TestRenameTagCasingOnly(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"go"}, "")

	if err := s.RenameTag("go", "Go"); err != nil {
		t.Fatal(err)
	}
	tags, _ := s.ListTags()
	if len(tags) != 1 || tags[0].Name != "Go" {
		t.Fatalf("tags = %+v, want one tag named Go", tags)
	}
}

func TestRenameTagMergesOnCollision(t *testing.T) {
	s := newTestStore(t)
	idA, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	idB, _ := s.UpsertBookmark(NewBookmark{URL: "https://b.com"})
	_ = s.SetBookmarkTags(idA, []string{"React"}, "")
	_ = s.SetBookmarkTags(idB, []string{"frontend"}, "")

	// Renaming React -> frontend collides; should merge into one tag.
	if err := s.RenameTag("React", "frontend"); err != nil {
		t.Fatal(err)
	}
	tags, _ := s.ListTags()
	if len(tags) != 1 {
		t.Fatalf("want 1 tag after merge, got %+v", tags)
	}
	a, _ := s.GetBookmark(idA)
	if len(a.Tags) != 1 || a.Tags[0] != "frontend" {
		t.Errorf("bookmark A tags = %v, want [frontend]", a.Tags)
	}
}
