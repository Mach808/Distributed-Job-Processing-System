package main

import "fmt"

func main() {
	jobs := make(chan string)

	go func() {
		fmt.Println("Worker: waiting for a job...")

		job := <-jobs

		fmt.Println("Worker received:", job)
	}()

	fmt.Println("Main: sending job...")

	jobs <- "Job 1"

	fmt.Println("Main: job sent")
}
