package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"time"
)

type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]Job
	db   *pgxpool.Pool
}

func NewJobStore(db *pgxpool.Pool) *JobStore {
	return &JobStore{
		jobs: make(map[string]Job),
		db:   db,
	}
}

func (s *JobStore) LoadJobsFromDB() error {
	rows, err := s.db.Query(
		context.Background(),
		`
		SELECT
			id,
			type,
			a,
			b,
			status,
			result,
			retries,
			worker_id,
			attempt,
			lease_until
		FROM jobs
		`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	for rows.Next() {
		var job Job
		var workerID pgtype.Text
		var leaseUntil pgtype.Timestamptz

		err := rows.Scan(
			&job.ID,
			&job.Type,
			&job.A,
			&job.B,
			&job.Status,
			&job.Result,
			&job.Retries,
			&workerID,
			&job.Attempt,
			&leaseUntil,
		)

		if err != nil {
			return err
		}

		if workerID.Valid {
			job.WorkerID = workerID.String
		}

		if leaseUntil.Valid {
			job.LeaseUntil = leaseUntil.Time
		}

		s.jobs[job.ID] = job
	}

	return rows.Err()
}

func (s *JobStore) Set(job Job) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.jobs[job.ID] = job

	_, err := s.db.Exec(
		context.Background(),
		`
		INSERT INTO jobs (
			id,
			type,
			a,
			b,
			status,
			result,
			retries,
			worker_id,
			attempt,
			lease_until
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id)
		DO UPDATE SET
			type = EXCLUDED.type,
			a = EXCLUDED.a,
			b = EXCLUDED.b,
			status = EXCLUDED.status,
			result = EXCLUDED.result,
			retries = EXCLUDED.retries,
			worker_id = EXCLUDED.worker_id,
			attempt = EXCLUDED.attempt,
			lease_until = EXCLUDED.lease_until
		`,
		job.ID,
		job.Type,
		job.A,
		job.B,
		job.Status,
		job.Result,
		job.Retries,
		job.WorkerID,
		job.Attempt,
		job.LeaseUntil,
	)

	if err != nil {
		fmt.Println("Database Set error:", err)
	}
}

func (s *JobStore) Get(id string) (Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, ok := s.jobs[id]

	return job, ok
}
func (s *JobStore) GetAll() []Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]Job, 0, len(s.jobs))

	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}

	return jobs
}

func (s *JobStore) RenewLease(id string, workerID string, attempt int, duration time.Duration) bool {
	newLease := time.Now().Add(duration)

	// PostgreSQL decides whether this worker still owns the job.
	tag, err := s.db.Exec(
		context.Background(),
		`
		UPDATE jobs
		SET lease_until = $1
		WHERE id = $2
		  AND status = 'processing'
		  AND worker_id = $3
		  AND attempt = $4
		`,
		newLease,
		id,
		workerID,
		attempt,
	)

	if err != nil {
		fmt.Println("Database RenewLease error:", err)
		return false
	}

	// Worker no longer owns the job.
	if tag.RowsAffected() == 0 {
		return false
	}

	// PostgreSQL accepted the renewal.
	// Synchronize the in-memory copy.
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return true
	}

	job.LeaseUntil = newLease
	s.jobs[id] = job

	return true
}

func (s *JobStore) Complete(id string, workerID string, attempt int, result int) bool {
	// PostgreSQL is now the authority for completion.
	tag, err := s.db.Exec(
		context.Background(),
		`
		UPDATE jobs
		SET
			status = 'completed',
			result = $1,
			worker_id = NULL,
			lease_until = NULL
		WHERE id = $2
		  AND status = 'processing'
		  AND worker_id = $3
		  AND attempt = $4
		`,
		result,
		id,
		workerID,
		attempt,
	)

	if err != nil {
		fmt.Println("Database Complete error:", err)
		return false
	}

	// No row was updated means this worker no longer owns the job.
	if tag.RowsAffected() == 0 {
		return false
	}

	// PostgreSQL accepted the completion.
	// Now synchronize the in-memory copy.
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return true
	}

	job.Status = "completed"
	job.Result = result
	job.WorkerID = ""
	job.LeaseUntil = time.Time{}

	s.jobs[id] = job

	return true
}

func (s *JobStore) Recover(id string) (Job, bool, error) {
	var job Job
	var workerID pgtype.Text
	var leaseUntil pgtype.Timestamptz

	err := s.db.QueryRow(
		context.Background(),
		`
		UPDATE jobs
		SET
			status = CASE
				WHEN retries + 1 >= 3 THEN 'failed'
				ELSE 'queued'
			END,
			retries = retries + 1,
			worker_id = NULL,
			lease_until = NULL
		WHERE id = $1
		  AND status = 'processing'
		  AND lease_until <= NOW()
		RETURNING
			id,
			type,
			a,
			b,
			status,
			COALESCE(result, 0),
			retries,
			worker_id,
			attempt,
			lease_until
		`,
		id,
	).Scan(
		&job.ID,
		&job.Type,
		&job.A,
		&job.B,
		&job.Status,
		&job.Result,
		&job.Retries,
		&workerID,
		&job.Attempt,
		&leaseUntil,
	)

	if err == pgx.ErrNoRows {
		return Job{}, false, nil
	}

	if err != nil {
		fmt.Println("Database Recover error:", err)
		return Job{}, false, err
	}

	if workerID.Valid {
		job.WorkerID = workerID.String
	}

	if leaseUntil.Valid {
		job.LeaseUntil = leaseUntil.Time
	}

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	return job, true, nil
}

func (s *JobStore) Claim(id string, workerID string) (Job, bool, error) {
	var job Job
	var worker string
	var leaseUntil time.Time

	err := s.db.QueryRow(
		context.Background(),
		`
		UPDATE jobs
		SET
			status = 'processing',
			worker_id = $1,
			attempt = attempt + 1,
			lease_until = NOW() + INTERVAL '10 seconds'
		WHERE id = $2
		  AND status = 'queued'
		RETURNING
			id,
			type,
			a,
			b,
			status,
			COALESCE(result, 0),
			retries,
			worker_id,
			attempt,
			lease_until
		`,
		workerID,
		id,
	).Scan(
		&job.ID,
		&job.Type,
		&job.A,
		&job.B,
		&job.Status,
		&job.Result,
		&job.Retries,
		&job.WorkerID,
		&job.Attempt,
		&leaseUntil,
	)

	if err == pgx.ErrNoRows {
		// Job is no longer queued.
		// Another worker may have already claimed it.
		return Job{}, false, nil
	}

	if err != nil {
		return Job{}, false, err
	}

	job.WorkerID = worker
	job.LeaseUntil = leaseUntil

	// Keep the temporary in-memory copy synchronized.
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	return job, true, nil
}
