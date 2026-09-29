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
			if store.Recover(job.ID) {
				fmt.Println("Recovering expired job:", job.ID)

				recoveredJob, ok := store.Get(job.ID)
				if ok {
					queue.Enqueue(recoveredJob)
				}
			}
		}
	}
}
