package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"sync"
	"time"
)

type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]Job
	db   *pgx.Conn
}

func NewJobStore(db *pgx.Conn) *JobStore {
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
		var leaseUntil pgtype.Timestamptz

		err := rows.Scan(
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
		if err != nil {
			return err
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

func (s *JobStore) RenewLease(id string, duration time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return false
	}

	if job.Status != "processing" {
		return false
	}

	newLease := time.Now().Add(duration)
	job.LeaseUntil = newLease

	s.jobs[id] = job

	_, err := s.db.Exec(
		context.Background(),
		`
		UPDATE jobs
		SET lease_until = $1
		WHERE id = $2
		  AND status = 'processing'
		`,
		newLease,
		id,
	)

	if err != nil {
		fmt.Println("Database RenewLease error:", err)
		return false
	}

	return true
}

func (s *JobStore) Complete(id string, workerID string, attempt int, result int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return false
	}

	if job.Status != "processing" {
		return false
	}

	if job.Attempt != attempt {
		return false
	}

	if job.WorkerID != workerID {
		return false
	}

	job.Result = result
	job.Status = "completed"
	job.WorkerID = ""
	job.LeaseUntil = time.Time{}

	s.jobs[id] = job

	_, err := s.db.Exec(
		context.Background(),
		`
		UPDATE jobs
		SET
			status = $1,
			result = $2,
			worker_id = $3,
			lease_until = $4
		WHERE id = $5
		`,
		job.Status,
		job.Result,
		job.WorkerID,
		nil,
		job.ID,
	)

	if err != nil {
		fmt.Println("Database Complete error:", err)
		return false
	}

	return true
}

func (s *JobStore) Recover(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return false
	}

	if job.Status != "processing" {
		return false
	}

	if time.Now().Before(job.LeaseUntil) {
		return false
	}

	job.Status = "queued"
	job.WorkerID = ""
	job.LeaseUntil = time.Time{}

	s.jobs[id] = job

	_, err := s.db.Exec(
		context.Background(),
		`
		UPDATE jobs
		SET
			status = $1,
			worker_id = $2,
			lease_until = $3
		WHERE id = $4
		  AND status = 'processing'
		  AND lease_until <= NOW()
		`,
		job.Status,
		nil,
		nil,
		job.ID,
	)

	if err != nil {
		fmt.Println("Database Recover error:", err)
		return false
	}

	return true
}
