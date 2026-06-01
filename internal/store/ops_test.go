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

func TestRetagBookmark(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"Go"}, "sum") // now tagged

	if err := s.RetagBookmark(id); err != nil {
		t.Fatal(err)
	}
	b, _ := s.GetBookmark(id)
	if b.Status != StatusPending {
		t.Errorf("status = %q, want pending", b.Status)
	}
	if _, ok, _ := s.ClaimNextJob(); !ok {
		t.Error("expected a re-enqueued job")
	}
}

func TestRetagAll(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	b, _ := s.UpsertBookmark(NewBookmark{URL: "https://b.com"})
	_ = s.SetBookmarkTags(a, []string{"x"}, "s") // both become tagged
	_ = s.SetBookmarkTags(b, []string{"y"}, "s")

	n, err := s.RetagAll()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("queued = %d, want 2", n)
	}
	for _, id := range []int64{a, b} {
		bm, _ := s.GetBookmark(id)
		if bm.Status != StatusPending {
			t.Errorf("bookmark %d status = %q, want pending", id, bm.Status)
		}
	}
	claimed := 0
	for {
		_, ok, _ := s.ClaimNextJob()
		if !ok {
			break
		}
		claimed++
	}
	if claimed != 2 {
		t.Errorf("claimable jobs = %d, want 2", claimed)
	}
}

func TestDeleteBookmarkPrunesOrphanTags(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.SetBookmarkTags(id, []string{"solo"}, "")

	if err := s.DeleteBookmark(id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBookmark(id); err == nil {
		t.Error("expected bookmark to be gone")
	}
	tags, _ := s.ListTags()
	if len(tags) != 0 {
		t.Errorf("expected orphan tag pruned, got %+v", tags)
	}
}
