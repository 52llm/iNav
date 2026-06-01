package tagger

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/52llm/iNav/internal/llm"
	"github.com/52llm/iNav/internal/store"
)

type fakeTagger struct {
	result llm.TagResult
	err    error
	seen   []string // ExistingTags passed in on the last call
}

func (f *fakeTagger) Tag(_ context.Context, in llm.TagInput) (llm.TagResult, error) {
	f.seen = in.ExistingTags
	return f.result, f.err
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProcessOneTagsBookmark(t *testing.T) {
	s := newStore(t)
	bID, _ := s.UpsertBookmark(store.NewBookmark{URL: "https://a.com", Title: "A"})
	_ = s.EnqueueTagJob(bID)

	w := New(s, &fakeTagger{result: llm.TagResult{Tags: []string{"Go"}, Summary: "sum"}})
	did, err := w.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !did {
		t.Fatal("expected ProcessOne to do work")
	}

	b, _ := s.GetBookmark(bID)
	if b.Status != store.StatusTagged {
		t.Errorf("status = %q, want tagged", b.Status)
	}
	if b.Summary != "sum" || len(b.Tags) != 1 {
		t.Errorf("tags/summary not applied: %+v", b)
	}
}

func TestProcessOneNoWork(t *testing.T) {
	s := newStore(t)
	w := New(s, &fakeTagger{})
	did, err := w.ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if did {
		t.Error("expected no work with empty queue")
	}
}

func TestProcessOneFailsJobOnLLMError(t *testing.T) {
	s := newStore(t)
	bID, _ := s.UpsertBookmark(store.NewBookmark{URL: "https://a.com"})
	_ = s.EnqueueTagJob(bID)

	w := New(s, &fakeTagger{err: errors.New("llm down")})
	if _, err := w.ProcessOne(context.Background()); err != nil {
		t.Fatalf("ProcessOne should swallow llm errors, got %v", err)
	}
	// Job requeued (attempts < max), bookmark still pending.
	b, _ := s.GetBookmark(bID)
	if b.Status != store.StatusPending {
		t.Errorf("status = %q, want pending after first failure", b.Status)
	}
}
