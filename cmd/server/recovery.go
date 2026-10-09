package main

import (
	"context"
	"log"
	"time"
)

const recoveryInterval = 2 * time.Second

// startRecovery atomically recovers expired leases in PostgreSQL. Recovered queued
// jobs become visible to all workers automatically; no local queue is involved.
func startRecovery(ctx context.Context, store *JobStore) {
	log.Println("lease recovery started")
	ticker := time.NewTicker(recoveryInterval)
	defer ticker.Stop()
	for {
		if err := store.RecoverExpired(ctx, maxRetries); err != nil {
			log.Printf("lease recovery failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
