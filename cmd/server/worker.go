package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

const (
	leaseDuration = 30 * time.Second
	pollInterval  = 500 * time.Millisecond
	maxRetries    = 3
)

type Worker struct {
	ID    string
	store *JobStore
}

func NewWorker(id string, store *JobStore) *Worker {
	return &Worker{ID: id, store: store}
}

// Start continuously claims jobs from PostgreSQL. The database is the only queue;
// workers never depend on process-local job state.
func (w *Worker) Start(ctx context.Context) {
	log.Printf("worker %s started", w.ID)
	for {
		if ctx.Err() != nil {
			return
		}

		job, claimed, err := w.store.ClaimNext(ctx, w.ID, leaseDuration)
		if err != nil {
			log.Printf("worker %s: claim job: %v", w.ID, err)
			if !sleepContext(ctx, pollInterval) {
				return
			}
			continue
		}
		if !claimed {
			if !sleepContext(ctx, pollInterval) {
				return
			}
			continue
		}

		w.process(ctx, job)
	}
}

func (w *Worker) process(parent context.Context, job Job) {
	ctx, cancel := context.WithCancel(parent)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(leaseDuration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ok, err := w.store.RenewLeaseContext(ctx, job.ID, w.ID, job.Attempt, leaseDuration)
				if err != nil {
					log.Printf("worker %s: renew lease for %s: %v", w.ID, job.ID, err)
					cancel()
					return
				}
				if !ok {
					log.Printf("worker %s lost ownership of job %s attempt %d", w.ID, job.ID, job.Attempt)
					cancel()
					return
				}
			}
		}
	}()

	result, err := runJobSafely(func() (int, error) {
		return executeJob(job)
	})
	cancel()
	<-heartbeatDone
	if parent.Err() != nil {
		return
	}
	if err != nil {
		failed, failErr := w.store.FailAttempt(parent, job.ID, w.ID, job.Attempt, maxRetries)
		if failErr != nil {
			log.Printf("worker %s: record failure for %s: %v; lease recovery will handle it", w.ID, job.ID, failErr)
			return
		}
		if failed {
			log.Printf("worker %s: job %s failed: %v", w.ID, job.ID, err)
		}
		return
	}

	completed, completeErr := w.store.CompleteContext(parent, job.ID, w.ID, job.Attempt, result)
	if completeErr != nil {
		log.Printf("worker %s: complete job %s: %v; lease recovery will handle it", w.ID, job.ID, completeErr)
		return
	}
	if completed {
		log.Printf("worker %s completed job %s result=%d", w.ID, job.ID, result)
	}
}

// runJobSafely contains panics raised while executing one job and turns them
// into ordinary attempt failures so the caller can apply the retry policy.
func runJobSafely(run func() (int, error)) (result int, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = 0
			err = fmt.Errorf("job processing panicked: %v", recovered)
		}
	}()
	return run()
}

func executeJob(job Job) (int, error) {
	switch job.Type {
	case "addition":
		return job.A + job.B, nil
	case "multiplication":
		return job.A * job.B, nil
	case "subtraction":
		return job.A - job.B, nil
	case "division":
		if job.B == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return job.A / job.B, nil
	case "fibonacci":
		if job.A < 0 || job.A > 46 {
			return 0, fmt.Errorf("fibonacci input must be between 0 and 46")
		}
		previous, current := 0, 1
		for i := 0; i < job.A; i++ {
			previous, current = current, previous+current
		}
		return previous, nil
	case "prime_count":
		if job.A < 2 || job.A > 10000000 {
			return 0, fmt.Errorf("prime_count input must be between 2 and 10000000")
		}
		return countPrimes(job.A), nil
	case "retry_demo":
		if job.A < 0 || job.A >= maxRetries {
			return 0, fmt.Errorf("retry_demo failure count must be between 0 and %d", maxRetries-1)
		}
		if job.Retries < job.A {
			return 0, fmt.Errorf("demo failure %d of %d", job.Retries+1, job.A)
		}
		return job.B, nil
	default:
		return 0, fmt.Errorf("unsupported job type %q", job.Type)
	}
}

func countPrimes(limit int) int {
	composite := make([]bool, limit+1)
	count := 0
	for candidate := 2; candidate <= limit; candidate++ {
		if composite[candidate] {
			continue
		}
		count++
		if candidate <= limit/candidate {
			for multiple := candidate * candidate; multiple <= limit; multiple += candidate {
				composite[multiple] = true
			}
		}
	}
	return count
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
