package store

import "database/sql"

const maxAttempts = 3

// Job is a queued tagging task for a bookmark.
type Job struct {
	ID         int64
	BookmarkID int64
	Status     string
	Attempts   int
}

// EnqueueTagJob adds a queued tagging job for the bookmark.
func (s *Store) EnqueueTagJob(bookmarkID int64) error {
	_, err := s.DB.Exec(
		`INSERT INTO jobs (bookmark_id, status) VALUES (?, ?)`,
		bookmarkID, JobQueued,
	)
	return err
}

// ClaimNextJob atomically marks the oldest queued job as running and returns it.
// ok is false when no job is available.
func (s *Store) ClaimNextJob() (Job, bool, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()

	var j Job
	err = tx.QueryRow(`
		SELECT id, bookmark_id, status, attempts FROM jobs
		WHERE status = ? ORDER BY id LIMIT 1`, JobQueued).
		Scan(&j.ID, &j.BookmarkID, &j.Status, &j.Attempts)
	if err == sql.ErrNoRows {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	if _, err := tx.Exec(
		`UPDATE jobs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		JobRunning, j.ID,
	); err != nil {
		return Job{}, false, err
	}
	j.Status = JobRunning
	return j, true, tx.Commit()
}

// CompleteJob marks a running job as done.
func (s *Store) CompleteJob(jobID int64) error {
	_, err := s.DB.Exec(
		`UPDATE jobs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		JobDone, jobID,
	)
	return err
}

// FailJob records an attempt failure. It requeues the job for retry until
// maxAttempts is reached, after which the job and its bookmark are marked failed.
func (s *Store) FailJob(jobID int64, errMsg string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var attempts, bookmarkID int64
	if err := tx.QueryRow(`SELECT attempts, bookmark_id FROM jobs WHERE id = ?`, jobID).
		Scan(&attempts, &bookmarkID); err != nil {
		return err
	}
	attempts++

	if attempts >= maxAttempts {
		if _, err := tx.Exec(
			`UPDATE jobs SET status = ?, attempts = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			JobFailed, attempts, errMsg, jobID,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE bookmarks SET status = ? WHERE id = ?`, StatusFailed, bookmarkID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(
			`UPDATE jobs SET status = ?, attempts = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			JobQueued, attempts, errMsg, jobID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
