package tagger

import (
	"context"
	"log"
	"time"

	"github.com/52llm/iNav/internal/llm"
	"github.com/52llm/iNav/internal/store"
)

// Worker polls the job queue and tags bookmarks via an llm.Tagger.
type Worker struct {
	store  *store.Store
	tagger llm.Tagger
}

// New builds a Worker.
func New(s *store.Store, t llm.Tagger) *Worker {
	return &Worker{store: s, tagger: t}
}

// ProcessOne claims at most one job and processes it. It reports whether work
// was done. LLM/processing failures are recorded against the job (and the job
// requeued or failed), not returned as errors.
func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	job, ok, err := w.store.ClaimNextJob()
	if err != nil || !ok {
		return false, err
	}

	b, err := w.store.GetBookmark(job.BookmarkID)
	if err != nil {
		_ = w.store.FailJob(job.ID, "load bookmark: "+err.Error())
		return true, nil
	}

	existing, err := w.store.ListTags()
	if err != nil {
		_ = w.store.FailJob(job.ID, "list tags: "+err.Error())
		return true, nil
	}
	names := make([]string, len(existing))
	for i, t := range existing {
		names[i] = t.Name
	}

	res, err := w.tagger.Tag(ctx, llm.TagInput{
		Title:        b.Title,
		URL:          b.URL,
		Excerpt:      b.Excerpt,
		Content:      b.Content,
		ExistingTags: names,
	})
	if err != nil {
		_ = w.store.FailJob(job.ID, err.Error())
		return true, nil
	}

	if err := w.store.SetBookmarkTags(job.BookmarkID, res.Tags, res.Summary); err != nil {
		_ = w.store.FailJob(job.ID, "set tags: "+err.Error())
		return true, nil
	}
	return true, w.store.CompleteJob(job.ID)
}

// Run loops ProcessOne until ctx is cancelled, sleeping when the queue is empty.
func (w *Worker) Run(ctx context.Context) {
	const idle = 2 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		did, err := w.ProcessOne(ctx)
		if err != nil {
			log.Printf("tagger: %v", err)
		}
		if !did {
			select {
			case <-ctx.Done():
				return
			case <-time.After(idle):
			}
		}
	}
}
