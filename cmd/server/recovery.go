package main

import (
	"fmt"
	"time"
)

const recoveryInterval = 5 * time.Second

func startRecovery(store *JobStore, queue *Queue) {
	fmt.Println("Recovery system started")

	for {
		time.Sleep(recoveryInterval)

		fmt.Println("Recovery check running")

		jobs := store.GetAll()

		for _, job := range jobs {
			if job.Status != "processing" {
				continue
			}
			recoveredJob, recovered, err := store.Recover(job.ID)

			if err != nil {
				fmt.Println("Recovery failed:", err)
				continue
			}
			if recovered {
				fmt.Println(
					"Recovered job:",
					recoveredJob.ID,
					"Status:",
					recoveredJob.Status,
					"Retries:",
					recoveredJob.Retries,
				)

				if recoveredJob.Status == "queued" {
					queue.Enqueue(recoveredJob)
				} else if recoveredJob.Status == "failed" {
					fmt.Println("Job permanently failed:", recoveredJob.ID)
				}
			}
		}
	}
}
