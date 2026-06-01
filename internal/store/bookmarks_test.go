package store

import "testing"

func TestUpsertBookmarkIsIdempotentByURL(t *testing.T) {
	s := newTestStore(t)

	id1, err := s.UpsertBookmark(NewBookmark{
		URL: "https://example.com", Title: "Example", Content: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.UpsertBookmark(NewBookmark{
		URL: "https://example.com", Title: "Example Updated", Content: "world",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("expected same id for same url, got %d and %d", id1, id2)
	}

	b, err := s.GetBookmark(id1)
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Example Updated" {
		t.Errorf("title = %q, want updated", b.Title)
	}
	if b.Status != StatusPending {
		t.Errorf("status = %q, want pending", b.Status)
	}
}

func TestSetBookmarkTagsAndList(t *testing.T) {
	s := newTestStore(t)
	id, err := s.UpsertBookmark(NewBookmark{URL: "https://a.com", Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBookmarkTags(id, []string{"Go", "web"}, "a one-line summary"); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListBookmarks(BookmarkFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(list))
	}
	got := list[0]
	if got.Status != StatusTagged {
		t.Errorf("status = %q, want tagged", got.Status)
	}
	if got.Summary != "a one-line summary" {
		t.Errorf("summary = %q", got.Summary)
	}
	if len(got.Tags) != 2 {
		t.Errorf("tags = %v, want 2", got.Tags)
	}
}

func TestListBookmarksFilterByTag(t *testing.T) {
	s := newTestStore(t)
	idA, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com", Title: "A"})
	idB, _ := s.UpsertBookmark(NewBookmark{URL: "https://b.com", Title: "B"})
	_ = s.SetBookmarkTags(idA, []string{"Go"}, "")
	_ = s.SetBookmarkTags(idB, []string{"Rust"}, "")

	list, err := s.ListBookmarks(BookmarkFilter{Tag: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].URL != "https://a.com" {
		t.Fatalf("tag filter failed: %+v", list)
	}
}
