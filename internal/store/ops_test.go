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

func TestMergeTags(t *testing.T) {
	s := newTestStore(t)
	idA, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	idB, _ := s.UpsertBookmark(NewBookmark{URL: "https://b.com"})
	_ = s.SetBookmarkTags(idA, []string{"React"}, "")
	_ = s.SetBookmarkTags(idB, []string{"Vue", "React"}, "")

	affected, err := s.MergeTags([]string{"React", "Vue"}, "frontend")
	if err != nil {
		t.Fatal(err)
	}
	if affected != 2 {
		t.Errorf("affected = %d, want 2", affected)
	}
	tags, _ := s.ListTags()
	if len(tags) != 1 || tags[0].Name != "frontend" {
		t.Fatalf("want only [frontend], got %+v", tags)
	}
	a, _ := s.GetBookmark(idA)
	if len(a.Tags) != 1 || a.Tags[0] != "frontend" {
		t.Errorf("A tags = %v", a.Tags)
	}
}

func TestAddAndRemoveTag(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"Go"}, "")

	n, err := s.AddTagToBookmarks([]int64{id}, "web")
	if err != nil || n != 1 {
		t.Fatalf("add: n=%d err=%v", n, err)
	}
	b, _ := s.GetBookmark(id)
	if len(b.Tags) != 2 {
		t.Fatalf("tags = %v, want 2", b.Tags)
	}

	n, err = s.RemoveTagFromBookmarks([]int64{id}, "web")
	if err != nil || n != 1 {
		t.Fatalf("remove: n=%d err=%v", n, err)
	}
	b, _ = s.GetBookmark(id)
	if len(b.Tags) != 1 || b.Tags[0] != "Go" {
		t.Errorf("tags = %v, want [Go]", b.Tags)
	}
	// "web" had no other bookmarks → pruned.
	tags, _ := s.ListTags()
	for _, tg := range tags {
		if tg.Name == "web" {
			t.Error("orphan tag 'web' was not pruned")
		}
	}
}
