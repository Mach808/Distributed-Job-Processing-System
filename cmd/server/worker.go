package main

import (
	"fmt"
	"time"
)

const (
	leaseDuration = 10 * time.Second
	maxRetries    = 3
)

type Worker struct {
	ID    string
	queue *Queue
	store *JobStore
}

func NewWorker(id string, queue *Queue, store *JobStore) *Worker {
	return &Worker{
		ID:    id,
		queue: queue,
		store: store,
	}
}

func (w *Worker) Start() {
	fmt.Println("Worker started")

	for {
		job := w.queue.Dequeue()

		success := w.safeProcess(job)

		if !success {
			job.Retries++

			fmt.Println("Job failed:", job.ID, "Retry:", job.Retries)

			if job.Retries < maxRetries {
				job.Status = "queued"
				job.LeaseUntil = time.Time{}
				job.WorkerID = ""

				w.store.Set(job)
				w.queue.Enqueue(job)
			} else {
				job.Status = "failed"
				job.LeaseUntil = time.Time{}
				job.WorkerID = ""

				w.store.Set(job)

				fmt.Println("Job permanently failed:", job.ID)
			}
		}
	}
}

func (w *Worker) safeProcess(job Job) (success bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Worker recovered from panic:", r)
			success = false
		}
	}()

	return w.process(job)
}

func (w *Worker) process(job Job) bool {
	fmt.Println("Processing job:", job.ID)

	claimedJob, claimed, err := w.store.Claim(job.ID, w.ID)

	if err != nil {
		fmt.Println("Failed to claim job:", err)
		return true
	}

	if !claimed {
		fmt.Println("Job was already claimed:", job.ID)
		return true
	}

	job = claimedJob

	stopHeartbeat := make(chan bool)
	defer close(stopHeartbeat)


	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if !w.store.RenewLease(job.ID, w.ID, job.Attempt, leaseDuration) {
					return
				}
				fmt.Println("Heartbeat:", job.ID)

			case <-stopHeartbeat:
				return
			}
		}
	}()

	var result int

	switch job.Type {

	case "addition":
//		time.Sleep(15 * time.Second)
		result = job.A + job.B

	case "multiplication":
		result = job.A * job.B

	default:
		job.Status = "failed"
		job.LeaseUntil = time.Time{}

		w.store.Set(job)

		return false
	}

	if w.store.Complete(job.ID, w.ID, job.Attempt, result) {
		fmt.Println(
			"I, worker -", w.ID,
			"have completed job with job ID", job.ID,
			"with result", result,
		)
	}

	return true
}
