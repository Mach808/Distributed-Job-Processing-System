package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func TestCreateRateLimitedRejectsEmptyBucketWithoutCreatingJob(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("load .env: %v", err)
	}
	db, err := connectDB()
	if err != nil {
		t.Fatalf("connectDB: %v", err)
	}
	defer db.Close()

	store := NewJobStore(db)
	ctx := context.Background()
	clientID := fmt.Sprintf("test-rate-client-%d", time.Now().UnixNano())
	firstJobID := fmt.Sprintf("test-rate-job-%d", time.Now().UnixNano())
	secondJobID := firstJobID + "-second"

	if _, err := db.Exec(ctx, `
		INSERT INTO api_clients (client_id, key_hash)
		VALUES ($1, $2)
	`, clientID, hashAPIKey(clientID)); err != nil {
		t.Fatalf("insert client: %v", err)
	}
	defer func() {
		if _, err := db.Exec(ctx, `DELETE FROM jobs WHERE id IN ($1, $2)`, firstJobID, secondJobID); err != nil {
			t.Errorf("delete test jobs: %v", err)
		}
		if _, err := db.Exec(ctx, `DELETE FROM api_clients WHERE client_id = $1`, clientID); err != nil {
			t.Errorf("delete test client: %v", err)
		}
	}()

	now := time.Now().UTC()
	first, err := store.CreateRateLimited(ctx, clientID, Job{ID: firstJobID, Type: "addition", A: 1, B: 2}, now)
	if err != nil || !first.Allowed {
		t.Fatalf("first job should be accepted: decision=%+v err=%v", first, err)
	}

	if _, err := db.Exec(ctx, `
		UPDATE rate_limit_buckets
		SET tokens = 0, last_refill_at = NOW()
		WHERE client_id = $1
	`, clientID); err != nil {
		t.Fatalf("empty bucket: %v", err)
	}

	second, err := store.CreateRateLimited(ctx, clientID, Job{ID: secondJobID, Type: "addition", A: 3, B: 4}, now)
	if err != nil {
		t.Fatalf("second job returned error: %v", err)
	}
	if second.Allowed || second.RetryAfterSecond < 1 {
		t.Fatalf("empty bucket should reject with retry delay: %+v", second)
	}

	var jobCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE id IN ($1, $2)`, firstJobID, secondJobID).Scan(&jobCount); err != nil {
		t.Fatalf("count test jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("expected only accepted job to be inserted, got %d", jobCount)
	}
}
