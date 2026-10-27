 package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const jobTokenCost = 1.0

type RateLimitDecision struct {
	Allowed          bool
	RemainingTokens  float64
	Capacity         float64
	RetryAfterSecond int
}

func formatRateLimitNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

// CreateRateLimited atomically refills a client's bucket, consumes one token,
// and inserts the job. A rejected request does not insert a job.
func (s *JobStore) CreateRateLimited(ctx context.Context, clientID string, job Job, requestedAt time.Time) (RateLimitDecision, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RateLimitDecision{}, fmt.Errorf("begin rate-limit transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Insert the candidate job before locking the bucket. This keeps the
	// bucket lock held only for the short refill/consume decision. Rejected
	// candidates are rolled back, while accepted candidates commit together
	// with the token update.
	_, err = tx.Exec(ctx, `
		INSERT INTO jobs (
			id, type, a, b, input_path, query, status,
			result, retries, worker_id, attempt, lease_until
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''),
			'queued', NULL, 0, NULL, 0, NULL)
	`, job.ID, job.Type, job.A, job.B, job.InputPath, job.Query)
	if err != nil {
		return RateLimitDecision{}, fmt.Errorf("create candidate job: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO rate_limit_buckets (client_id)
		VALUES ($1)
		ON CONFLICT (client_id) DO NOTHING
	`, clientID)
	if err != nil {
		return RateLimitDecision{}, fmt.Errorf("initialize rate-limit bucket: %w", err)
	}

	var tokens, capacity, refillRate float64
	var lastRefillAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT tokens::double precision,
		       capacity::double precision,
		       refill_rate::double precision,
		       last_refill_at
		FROM rate_limit_buckets
		WHERE client_id = $1
		FOR UPDATE
	`, clientID).Scan(&tokens, &capacity, &refillRate, &lastRefillAt)
	if err != nil {
		return RateLimitDecision{}, fmt.Errorf("lock rate-limit bucket: %w", err)
	}

	// Use the request's arrival time rather than transaction start time. The
	// latter can be delayed by the pgx pool, making one client burst appear to
	// arrive over many seconds and incorrectly allowing every request.
	effectiveNow := requestedAt
	if effectiveNow.Before(lastRefillAt) {
		effectiveNow = lastRefillAt
	}
	elapsed := effectiveNow.Sub(lastRefillAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	refilled := math.Min(capacity, tokens+elapsed*refillRate)
	decision := RateLimitDecision{
		Allowed:         refilled >= jobTokenCost,
		RemainingTokens: refilled,
		Capacity:        capacity,
	}
	if decision.Allowed {
		decision.RemainingTokens -= jobTokenCost
	} else {
		seconds := math.Ceil((jobTokenCost - refilled) / refillRate)
		if seconds < 1 {
			seconds = 1
		}
		decision.RetryAfterSecond = int(seconds)
	}

	_, err = tx.Exec(ctx, `
		UPDATE rate_limit_buckets
		SET tokens = $1, last_refill_at = $2, updated_at = NOW()
		WHERE client_id = $3
	`, decision.RemainingTokens, effectiveNow, clientID)
	if err != nil {
		return RateLimitDecision{}, fmt.Errorf("update rate-limit bucket: %w", err)
	}

	if !decision.Allowed {
		if err := tx.Rollback(ctx); err != nil {
			return RateLimitDecision{}, fmt.Errorf("rollback rate-limited job: %w", err)
		}
		return decision, nil
	}

	if err := tx.Commit(ctx); err != nil {
		return RateLimitDecision{}, fmt.Errorf("commit rate-limited job: %w", err)
	}
	return decision, nil
}
