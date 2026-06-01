package store

import "testing"

func TestEnqueueAndClaimJob(t *testing.T) {
	s := newTestStore(t)
	bID, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	if err := s.EnqueueTagJob(bID); err != nil {
		t.Fatal(err)
	}

	job, ok, err := s.ClaimNextJob()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a job to claim")
	}
	if job.BookmarkID != bID {
		t.Errorf("BookmarkID = %d, want %d", job.BookmarkID, bID)
	}
	if job.Status != JobRunning {
		t.Errorf("claimed job status = %q, want running", job.Status)
	}

	// A second claim finds nothing (the only job is now running).
	if _, ok, _ := s.ClaimNextJob(); ok {
		t.Error("expected no claimable job")
	}
}

func TestCompleteJob(t *testing.T) {
	s := newTestStore(t)
	bID, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.EnqueueTagJob(bID)
	job, _, _ := s.ClaimNextJob()

	if err := s.CompleteJob(job.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = s.DB.QueryRow(`SELECT status FROM jobs WHERE id = ?`, job.ID).Scan(&status)
	if status != JobDone {
		t.Errorf("job status = %q, want done", status)
	}
}

func TestFailJobRequeuesUntilMaxAttempts(t *testing.T) {
	s := newTestStore(t)
	bID, _ := s.UpsertBookmark(NewBookmark{URL: "https://a.com"})
	_ = s.EnqueueTagJob(bID)

	// Fail it maxAttempts times; it should requeue until the cap, then mark failed.
	for i := 0; i < maxAttempts; i++ {
		job, ok, err := s.ClaimNextJob()
		if err != nil || !ok {
			t.Fatalf("attempt %d: claim failed ok=%v err=%v", i, ok, err)
		}
		if err := s.FailJob(job.ID, "boom"); err != nil {
			t.Fatal(err)
		}
	}
	// After maxAttempts failures there should be no claimable job left.
	if _, ok, _ := s.ClaimNextJob(); ok {
		t.Error("expected job to be terminally failed, but it was claimable")
	}
	var jobStatus, bmStatus string
	_ = s.DB.QueryRow(`SELECT status FROM jobs WHERE bookmark_id = ?`, bID).Scan(&jobStatus)
	_ = s.DB.QueryRow(`SELECT status FROM bookmarks WHERE id = ?`, bID).Scan(&bmStatus)
	if jobStatus != JobFailed {
		t.Errorf("job status = %q, want failed", jobStatus)
	}
	if bmStatus != StatusFailed {
		t.Errorf("bookmark status = %q, want failed", bmStatus)
	}
}
