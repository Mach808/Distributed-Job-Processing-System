package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"sync"
	"testing"
	"time"
)

func TestConcurrentClaim(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}
	db, err := connectDB()
	if err != nil {
		t.Fatalf("connectDB failed: %v", err)
	}
	defer db.Close()

	store := NewJobStore(db)

	jobID := fmt.Sprintf("test-claim-%d", time.Now().UnixNano())

	// Create a unique test job in PostgreSQL.
	_, err = db.Exec(
		context.Background(),
		`
		INSERT INTO jobs (id, type, a, b, status, retries, attempt)
		VALUES ($1, 'addition', 10, 20, 'queued', 0, 0)
		`,
		jobID,
	)
	if err != nil {
		t.Fatalf("insert test job failed: %v", err)
	}

	// Always clean up this test job.
	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM jobs WHERE id = $1`,
			jobID,
		)
		if err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	results := make(chan bool, 2)
	errors := make(chan error, 2)

	for _, workerID := range []string{"test-worker-1", "test-worker-2"} {
		go func(workerID string) {
			defer wg.Done()

			_, claimed, err := store.Claim(jobID, workerID)
			if err != nil {
				errors <- err
				return
			}

			results <- claimed
		}(workerID)
	}

	wg.Wait()
	close(results)
	close(errors)

	for err := range errors {
		t.Errorf("claim returned error: %v", err)
	}

	successCount := 0
	for claimed := range results {
		if claimed {
			successCount++
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful claim, got %d", successCount)
	}
}

func TestStaleWorkerCannotComplete(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	db, err := connectDB()
	if err != nil {
		t.Fatalf("connectDB failed: %v", err)
	}
	defer db.Close()

	store := NewJobStore(db)
	jobID := fmt.Sprintf("test-fence-%d", time.Now().UnixNano())

	_, err = db.Exec(
		context.Background(),
		`
		INSERT INTO jobs (id, type, a, b, status, retries, attempt)
		VALUES ($1, 'addition', 10, 20, 'queued', 0, 0)
		`,
		jobID,
	)
	if err != nil {
		t.Fatalf("insert test job failed: %v", err)
	}

	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM jobs WHERE id = $1`,
			jobID,
		)
		if err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// Worker A claims attempt 1.
	_, claimed, err := store.Claim(jobID, "worker-A")
	if err != nil || !claimed {
		t.Fatalf("worker A failed to claim: claimed=%v err=%v", claimed, err)
	}

	// Simulate expiration and recovery.
	_, err = db.Exec(
		context.Background(),
		`UPDATE jobs SET lease_until = NOW() - INTERVAL '1 second' WHERE id = $1`,
		jobID,
	)
	if err != nil {
		t.Fatalf("expire lease failed: %v", err)
	}

	if store.RenewLease(jobID, "worker-A", 1, 10*time.Second) {
		t.Fatal("expired worker A was incorrectly allowed to renew its lease")
	}
	if store.Complete(jobID, "worker-A", 1, 999) {
		t.Fatal("expired worker A was incorrectly allowed to complete the job")
	}

	_, recovered, err := store.Recover(jobID)
	if err != nil || !recovered {
		t.Fatalf("recovery failed: recovered=%v err=%v", recovered, err)
	}

	// Worker B claims attempt 2.
	claimedJob, claimed, err := store.Claim(jobID, "worker-B")
	if err != nil || !claimed {
		t.Fatalf("worker B failed to claim: claimed=%v err=%v", claimed, err)
	}

	if claimedJob.Attempt != 2 {
		t.Fatalf("expected attempt 2, got %d", claimedJob.Attempt)
	}

	// Worker A tries to complete its stale attempt 1.
	accepted := store.Complete(jobID, "worker-A", 1, 999)

	if accepted {
		t.Fatal("stale worker A was incorrectly allowed to complete the job")
	}

	// Confirm the database still records worker B's attempt.
	var status string
	var workerID string
	var attempt int

	err = db.QueryRow(
		context.Background(),
		`SELECT status, worker_id, attempt FROM jobs WHERE id = $1`,
		jobID,
	).Scan(&status, &workerID, &attempt)
	if err != nil {
		t.Fatalf("query final job state failed: %v", err)
	}

	if status != "processing" || workerID != "worker-B" || attempt != 2 {
		t.Fatalf(
			"unexpected job state: status=%s worker=%s attempt=%d",
			status, workerID, attempt,
		)
	}
}

func TestLeaseRenewalFencing(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	db, err := connectDB()
	if err != nil {
		t.Fatalf("connectDB failed: %v", err)
	}
	defer db.Close()

	store := NewJobStore(db)
	jobID := fmt.Sprintf("test-lease-%d", time.Now().UnixNano())

	_, err = db.Exec(
		context.Background(),
		`
		INSERT INTO jobs (id, type, a, b, status, retries, attempt)
		VALUES ($1, 'addition', 10, 20, 'queued', 0, 0)
		`,
		jobID,
	)
	if err != nil {
		t.Fatalf("insert test job failed: %v", err)
	}

	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM jobs WHERE id = $1`,
			jobID,
		)
		if err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// Worker A claims attempt 1.
	_, claimed, err := store.Claim(jobID, "worker-A")
	if err != nil || !claimed {
		t.Fatalf(
			"worker A failed to claim: claimed=%v err=%v",
			claimed, err,
		)
	}

	// A valid owner should be able to renew its lease.
	if !store.RenewLease(jobID, "worker-A", 1, 10*time.Second) {
		t.Fatal("valid worker A could not renew its lease")
	}

	// Simulate lease expiration, then recover the job.
	_, err = db.Exec(
		context.Background(),
		`UPDATE jobs SET lease_until = NOW() - INTERVAL '1 second' WHERE id = $1`,
		jobID,
	)
	if err != nil {
		t.Fatalf("expire lease failed: %v", err)
	}

	_, recovered, err := store.Recover(jobID)
	if err != nil || !recovered {
		t.Fatalf(
			"recovery failed: recovered=%v err=%v",
			recovered, err,
		)
	}

	// Worker B claims attempt 2.
	claimedJob, claimed, err := store.Claim(jobID, "worker-B")
	if err != nil || !claimed {
		t.Fatalf(
			"worker B failed to claim: claimed=%v err=%v",
			claimed, err,
		)
	}

	if claimedJob.Attempt != 2 {
		t.Fatalf("expected attempt 2, got %d", claimedJob.Attempt)
	}

	// Worker A's stale heartbeat must be rejected.
	if store.RenewLease(jobID, "worker-A", 1, 10*time.Second) {
		t.Fatal("stale worker A was incorrectly allowed to renew its lease")
	}

	// Confirm PostgreSQL still records worker B's ownership.
	var status string
	var workerID string
	var attempt int

	err = db.QueryRow(
		context.Background(),
		`SELECT status, worker_id, attempt FROM jobs WHERE id = $1`,
		jobID,
	).Scan(&status, &workerID, &attempt)
	if err != nil {
		t.Fatalf("query final job state failed: %v", err)
	}

	if status != "processing" || workerID != "worker-B" || attempt != 2 {
		t.Fatalf(
			"unexpected job state: status=%s worker=%s attempt=%d",
			status, workerID, attempt,
		)
	}
}

func TestLegacyCompleteAcceptsActiveLease(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	db, err := connectDB()
	if err != nil {
		t.Fatalf("connectDB failed: %v", err)
	}
	defer db.Close()

	store := NewJobStore(db)
	jobID := fmt.Sprintf("test-active-complete-%d", time.Now().UnixNano())
	_, err = db.Exec(context.Background(), `
		INSERT INTO jobs (id, type, a, b, status, retries, attempt)
		VALUES ($1, 'addition', 10, 20, 'queued', 0, 0)
	`, jobID)
	if err != nil {
		t.Fatalf("insert test job failed: %v", err)
	}
	defer func() {
		if _, err := db.Exec(context.Background(), `DELETE FROM jobs WHERE id = $1`, jobID); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	if _, claimed, err := store.Claim(jobID, "active-worker"); err != nil || !claimed {
		t.Fatalf("claim failed: claimed=%v err=%v", claimed, err)
	}
	if !store.Complete(jobID, "active-worker", 1, 30) {
		t.Fatal("active lease owner could not complete the job")
	}

	var status string
	var result int
	if err := db.QueryRow(context.Background(), `SELECT status, result FROM jobs WHERE id = $1`, jobID).Scan(&status, &result); err != nil {
		t.Fatalf("query completed job failed: %v", err)
	}
	if status != "completed" || result != 30 {
		t.Fatalf("unexpected completed job: status=%s result=%d", status, result)
	}
}

func TestRecoverExpiredLease(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	db, err := connectDB()
	if err != nil {
		t.Fatalf("connectDB failed: %v", err)
	}
	defer db.Close()

	store := NewJobStore(db)
	jobID := fmt.Sprintf("test-recovery-%d", time.Now().UnixNano())

	_, err = db.Exec(
		context.Background(),
		`
		INSERT INTO jobs (id, type, a, b, status, retries, attempt)
		VALUES ($1, 'addition', 10, 20, 'queued', 0, 0)
		`,
		jobID,
	)
	if err != nil {
		t.Fatalf("insert test job failed: %v", err)
	}

	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM jobs WHERE id = $1`,
			jobID,
		)
		if err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// Claim the job so it enters processing.
	_, claimed, err := store.Claim(jobID, "worker-A")
	if err != nil || !claimed {
		t.Fatalf("claim failed: claimed=%v err=%v", claimed, err)
	}

	// First, verify a valid lease is not recovered.
	_, recovered, err := store.Recover(jobID)
	if err != nil {
		t.Fatalf("recovery check failed: %v", err)
	}
	if recovered {
		t.Fatal("job with a valid lease was incorrectly recovered")
	}

	// Now force the lease to expire in PostgreSQL.
	_, err = db.Exec(
		context.Background(),
		`UPDATE jobs SET lease_until = NOW() - INTERVAL '1 second' WHERE id = $1`,
		jobID,
	)
	if err != nil {
		t.Fatalf("expire lease failed: %v", err)
	}

	// Recovery should now succeed.
	recoveredJob, recovered, err := store.Recover(jobID)
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	if !recovered {
		t.Fatal("expired job was not recovered")
	}

	if recoveredJob.Status != "queued" {
		t.Fatalf("expected status queued, got %s", recoveredJob.Status)
	}

	if recoveredJob.WorkerID != "" {
		t.Fatalf("expected empty worker ID, got %s", recoveredJob.WorkerID)
	}

	if !recoveredJob.LeaseUntil.IsZero() {
		t.Fatal("expected cleared lease after recovery")
	}
}

func TestRecoveryRetryLimit(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	db, err := connectDB()
	if err != nil {
		t.Fatalf("connectDB failed: %v", err)
	}
	defer db.Close()

	store := NewJobStore(db)
	jobID := fmt.Sprintf("test-retry-limit-%d", time.Now().UnixNano())

	_, err = db.Exec(
		context.Background(),
		`
		INSERT INTO jobs (id, type, a, b, status, retries, attempt)
		VALUES ($1, 'addition', 10, 20, 'queued', 0, 0)
		`,
		jobID,
	)
	if err != nil {
		t.Fatalf("insert test job failed: %v", err)
	}

	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM jobs WHERE id = $1`,
			jobID,
		)
		if err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	// Simulate repeated lease expirations.
	for expectedRetry := 1; expectedRetry <= 3; expectedRetry++ {
		_, claimed, err := store.Claim(jobID, "test-worker")
		if err != nil || !claimed {
			t.Fatalf(
				"claim %d failed: claimed=%v err=%v",
				expectedRetry, claimed, err,
			)
		}

		_, err = db.Exec(
			context.Background(),
			`UPDATE jobs SET lease_until = NOW() - INTERVAL '1 second' WHERE id = $1`,
			jobID,
		)
		if err != nil {
			t.Fatalf("expire lease %d failed: %v", expectedRetry, err)
		}

		recoveredJob, recovered, err := store.Recover(jobID)
		if err != nil {
			t.Fatalf("recovery %d failed: %v", expectedRetry, err)
		}
		if !recovered {
			t.Fatalf("expected recovery %d to succeed", expectedRetry)
		}

		if recoveredJob.Retries != expectedRetry {
			t.Fatalf(
				"expected retries=%d, got %d",
				expectedRetry, recoveredJob.Retries,
			)
		}

		if expectedRetry < 3 && recoveredJob.Status != "queued" {
			t.Fatalf(
				"expected queued after retry %d, got %s",
				expectedRetry, recoveredJob.Status,
			)
		}

		if expectedRetry == 3 && recoveredJob.Status != "failed" {
			t.Fatalf(
				"expected failed after retry limit, got %s",
				recoveredJob.Status,
			)
		}

		// Once recovery marks the job failed, no further claim should succeed.
		if expectedRetry == 3 {
			_, claimedAgain, err := store.Claim(jobID, "another-worker")
			if err != nil {
				t.Fatalf("claim after failure returned error: %v", err)
			}
			if claimedAgain {
				t.Fatal("failed job was incorrectly claimed again")
			}
		}
	}
}

func TestClaimNextAndFailAttemptLifecycle(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}
	db, err := connectDB()
	if err != nil {
		t.Fatalf("connectDB failed: %v", err)
	}
	defer db.Close()

	store := NewJobStore(db)
	ctx := context.Background()
	jobID := fmt.Sprintf("!test-claim-next-%d", time.Now().UnixNano())
	_, err = db.Exec(ctx, `INSERT INTO jobs (id, type, a, b, status, retries, attempt) VALUES ($1, 'addition', 2, 3, 'queued', 0, 0)`, jobID)
	if err != nil {
		t.Fatalf("insert test job: %v", err)
	}
	defer func() {
		if _, err := db.Exec(context.Background(), `DELETE FROM jobs WHERE id = $1`, jobID); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	claimed, ok, err := store.ClaimNext(ctx, "lifecycle-worker", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("ClaimNext: claimed=%v err=%v", ok, err)
	}
	if claimed.ID != jobID || claimed.Attempt != 1 || claimed.WorkerID != "lifecycle-worker" {
		t.Fatalf("unexpected claimed job: %+v", claimed)
	}

	// First failure must return the job to the queue.
	failed, err := store.FailAttempt(ctx, jobID, "lifecycle-worker", 1, 3)
	if err != nil || !failed {
		t.Fatalf("first FailAttempt: updated=%v err=%v", failed, err)
	}
	job, found, err := store.GetContext(ctx, jobID)
	if err != nil || !found {
		t.Fatalf("GetContext after first failure: found=%v err=%v", found, err)
	}
	if job.Status != "queued" || job.Retries != 1 || job.WorkerID != "" {
		t.Fatalf("unexpected state after first failure: %+v", job)
	}

	// A stale attempt must not be allowed to fail a newer claim.
	claimed, ok, err = store.ClaimNext(ctx, "replacement-worker", 30*time.Second)
	if err != nil || !ok || claimed.Attempt != 2 {
		t.Fatalf("second ClaimNext: job=%+v claimed=%v err=%v", claimed, ok, err)
	}
	updated, err := store.FailAttempt(ctx, jobID, "lifecycle-worker", 1, 3)
	if err != nil {
		t.Fatalf("stale FailAttempt returned error: %v", err)
	}
	if updated {
		t.Fatal("stale worker was allowed to fail a newer attempt")
	}

	// Expire the current attempt and verify batch recovery makes it queued.
	if _, err := db.Exec(ctx, `UPDATE jobs SET lease_until = NOW() - INTERVAL '1 second' WHERE id = $1`, jobID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	if err := store.RecoverExpired(ctx, 3); err != nil {
		t.Fatalf("RecoverExpired: %v", err)
	}
	job, found, err = store.GetContext(ctx, jobID)
	if err != nil || !found {
		t.Fatalf("GetContext after recovery: found=%v err=%v", found, err)
	}
	if job.Status != "queued" || job.Retries != 2 || job.WorkerID != "" || !job.LeaseUntil.IsZero() {
		t.Fatalf("unexpected state after batch recovery: %+v", job)
	}
}

func TestRecoverExpiredEnforcesRetryLimit(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}
	db, err := connectDB()
	if err != nil {
		t.Fatalf("connectDB failed: %v", err)
	}
	defer db.Close()

	store := NewJobStore(db)
	ctx := context.Background()
	jobID := fmt.Sprintf("test-batch-limit-%d", time.Now().UnixNano())
	_, err = db.Exec(ctx, `INSERT INTO jobs (id, type, a, b, status, retries, attempt, worker_id, lease_until) VALUES ($1, 'addition', 2, 3, 'processing', 2, 3, 'crashed-worker', NOW() - INTERVAL '1 second')`, jobID)
	if err != nil {
		t.Fatalf("insert expired job: %v", err)
	}
	defer func() {
		if _, err := db.Exec(context.Background(), `DELETE FROM jobs WHERE id = $1`, jobID); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	if err := store.RecoverExpired(ctx, 3); err != nil {
		t.Fatalf("RecoverExpired: %v", err)
	}
	job, found, err := store.GetContext(ctx, jobID)
	if err != nil || !found {
		t.Fatalf("GetContext: found=%v err=%v", found, err)
	}
	if job.Status != "failed" || job.Retries != 3 || job.WorkerID != "" || !job.LeaseUntil.IsZero() {
		t.Fatalf("retry limit not enforced by batch recovery: %+v", job)
	}
}
