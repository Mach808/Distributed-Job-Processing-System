package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"time"
)

type JobStore struct {
	db *pgxpool.Pool
}

func NewJobStore(db *pgxpool.Pool) *JobStore {
	return &JobStore{db: db}
}

func (s *JobStore) Get(id string) (Job, bool) {
	var job Job
	var workerID pgtype.Text
	var leaseUntil pgtype.Timestamptz

	err := s.db.QueryRow(context.Background(), `
		SELECT id, type, a, b, status,
		       COALESCE(result, 0), retries,
		       worker_id, attempt, lease_until
		FROM jobs
		WHERE id = $1
	`, id).Scan(
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
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, false
		}

		log.Printf("Failed to get job %s from database: %v", id, err)
		return Job{}, false
	}

	if workerID.Valid {
		job.WorkerID = workerID.String
	}

	if leaseUntil.Valid {
		job.LeaseUntil = leaseUntil.Time
	}

	return job, true
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
		  AND lease_until > NOW()
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
		  AND lease_until > NOW()
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

	return job, true, nil
}

func (s *JobStore) GetExpiredJobs() ([]Job, error) {
	rows, err := s.db.Query(context.Background(), `
		SELECT id, type, a, b, status,
		       COALESCE(result, 0), retries,
		       worker_id, attempt, lease_until
		FROM jobs
		WHERE status = 'processing'
		  AND lease_until <= NOW()
	`)
	if err != nil {
		return nil, fmt.Errorf("query expired jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job

	for rows.Next() {
		var job Job
		var workerID pgtype.Text
		var leaseUntil pgtype.Timestamptz

		if err := rows.Scan(
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
		); err != nil {
			return nil, fmt.Errorf("scan expired job: %w", err)
		}

		if workerID.Valid {
			job.WorkerID = workerID.String
		}
		if leaseUntil.Valid {
			job.LeaseUntil = leaseUntil.Time
		}

		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired jobs: %w", err)
	}

	return jobs, nil
}

func (s *JobStore) GetQueuedJobs() ([]Job, error) {
	rows, err := s.db.Query(context.Background(), `
		SELECT id, type, a, b, status,
		       COALESCE(result, 0), retries,
		       worker_id, attempt, lease_until
		FROM jobs
		WHERE status = 'queued'
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("query queued jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job

	for rows.Next() {
		var job Job
		var workerID pgtype.Text
		var leaseUntil pgtype.Timestamptz

		if err := rows.Scan(
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
		); err != nil {
			return nil, fmt.Errorf("scan queued job: %w", err)
		}

		if workerID.Valid {
			job.WorkerID = workerID.String
		}
		if leaseUntil.Valid {
			job.LeaseUntil = leaseUntil.Time
		}

		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queued jobs: %w", err)
	}

	return jobs, nil
}

func (s *JobStore) Create(job Job) error {
	_, err := s.db.Exec(context.Background(), `
		INSERT INTO jobs (
			id, type, a, b, status, result,
			retries, worker_id, attempt, lease_until
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		job.ID,
		job.Type,
		job.A,
		job.B,
		job.Status,
		job.Result,
		job.Retries,
		nil,
		job.Attempt,
		nil,
	)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}

	return nil
}

func (s *JobStore) ClaimNext(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) (Job, bool, error) {
	leaseSeconds := int(leaseDuration.Seconds())
	if workerID == "" {
		return Job{}, false, fmt.Errorf("worker ID cannot be empty")
	}
	if leaseSeconds <= 0 {
		return Job{}, false, fmt.Errorf("lease duration must be at least one second")
	}

	var job Job

	err := s.db.QueryRow(ctx, `
        WITH next_job AS (
            SELECT id
            FROM jobs
            WHERE status = 'queued'
            ORDER BY id
            FOR UPDATE SKIP LOCKED
            LIMIT 1
        )
        UPDATE jobs AS j
        SET
            status = 'processing',
            worker_id = $1,
            attempt = j.attempt + 1,
            lease_until = NOW() + ($2::integer * INTERVAL '1 second')
        FROM next_job
        WHERE j.id = next_job.id
		RETURNING
			j.id,
            j.type,
            j.a,
            j.b,
			COALESCE(j.input_path, ''),
			COALESCE(j.query, ''),
            j.status,
            COALESCE(j.result, 0),
            j.retries,
            j.worker_id,
            j.attempt,
            j.lease_until
    `, workerID, leaseSeconds).Scan(
		&job.ID,
		&job.Type,
		&job.A,
		&job.B,
		&job.InputPath,
		&job.Query,
		&job.Status,
		&job.Result,
		&job.Retries,
		&job.WorkerID,
		&job.Attempt,
		&job.LeaseUntil,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, fmt.Errorf("claim next job: %w", err)
	}

	return job, true, nil
}

func (s *JobStore) CreateContext(ctx context.Context, job Job) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO jobs (id, type, a, b, input_path, query, status, result, retries, worker_id, attempt, lease_until)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), 'queued', NULL, 0, NULL, 0, NULL)
	`, job.ID, job.Type, job.A, job.B, job.InputPath, job.Query)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

func (s *JobStore) GetContext(ctx context.Context, id string) (Job, bool, error) {
	var job Job
	var workerID pgtype.Text
	var leaseUntil pgtype.Timestamptz
	err := s.db.QueryRow(ctx, `
		SELECT id, type, a, b, status, COALESCE(result, 0), retries,
		       worker_id, attempt, lease_until
		FROM jobs WHERE id = $1
	`, id).Scan(&job.ID, &job.Type, &job.A, &job.B, &job.Status,
		&job.Result, &job.Retries, &workerID, &job.Attempt, &leaseUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, fmt.Errorf("get job: %w", err)
	}
	if workerID.Valid {
		job.WorkerID = workerID.String
	}
	if leaseUntil.Valid {
		job.LeaseUntil = leaseUntil.Time
	}
	return job, true, nil
}

func (s *JobStore) ListRecentContext(ctx context.Context, limit int) ([]Job, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, type, a, b, status, COALESCE(result, 0), retries,
		       worker_id, attempt, lease_until
		FROM jobs
		ORDER BY id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]Job, 0)
	for rows.Next() {
		var job Job
		var workerID pgtype.Text
		var leaseUntil pgtype.Timestamptz
		if err := rows.Scan(&job.ID, &job.Type, &job.A, &job.B, &job.Status,
			&job.Result, &job.Retries, &workerID, &job.Attempt, &leaseUntil); err != nil {
			return nil, fmt.Errorf("scan listed job: %w", err)
		}
		if workerID.Valid {
			job.WorkerID = workerID.String
		}
		if leaseUntil.Valid {
			job.LeaseUntil = leaseUntil.Time
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate listed jobs: %w", err)
	}
	return jobs, nil
}

func (s *JobStore) RenewLeaseContext(ctx context.Context, id, workerID string, attempt int, duration time.Duration) (bool, error) {
	seconds := int(duration.Seconds())
	if seconds < 1 {
		return false, fmt.Errorf("lease duration must be at least one second")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE jobs SET lease_until = NOW() + ($1::integer * INTERVAL '1 second')
		WHERE id = $2 AND status = 'processing' AND worker_id = $3
		  AND attempt = $4 AND lease_until > NOW()
	`, seconds, id, workerID, attempt)
	if err != nil {
		return false, fmt.Errorf("renew lease: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *JobStore) CompleteContext(ctx context.Context, id, workerID string, attempt, result int) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE jobs SET status = 'completed', result = $1,
			worker_id = NULL, lease_until = NULL
		WHERE id = $2 AND status = 'processing' AND worker_id = $3
		  AND attempt = $4 AND lease_until > NOW()
	`, result, id, workerID, attempt)
	if err != nil {
		return false, fmt.Errorf("complete job: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// FailAttempt atomically transitions the current owned attempt to queued or failed.
// A stale worker cannot alter a newer attempt because owner and attempt are fenced.
func (s *JobStore) FailAttempt(ctx context.Context, id, workerID string, attempt, retryLimit int) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE jobs SET
			status = CASE WHEN retries + 1 >= $1 THEN 'failed' ELSE 'queued' END,
			retries = retries + 1, worker_id = NULL, lease_until = NULL
		WHERE id = $2 AND status = 'processing' AND worker_id = $3
		  AND attempt = $4 AND lease_until > NOW()
	`, retryLimit, id, workerID, attempt)
	if err != nil {
		return false, fmt.Errorf("fail job attempt: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// RecoverExpired atomically recovers every expired processing lease. Repeated
// recovery loops and multiple server instances are safe because each update is conditional.
func (s *JobStore) RecoverExpired(ctx context.Context, retryLimit int) error {
	_, err := s.db.Exec(ctx, `
		UPDATE jobs SET
			status = CASE WHEN retries + 1 >= $1 THEN 'failed' ELSE 'queued' END,
			retries = retries + 1, worker_id = NULL, lease_until = NULL
		WHERE status = 'processing' AND lease_until <= NOW()
	`, retryLimit)
	if err != nil {
		return fmt.Errorf("recover expired jobs: %w", err)
	}
	return nil
}
