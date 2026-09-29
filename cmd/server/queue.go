package main

type Queue struct {
	jobs chan Job
}

func NewQueue(size int) *Queue {
	return &Queue{
		jobs: make(chan Job, size),
	}
}

func (q *Queue) Enqueue(job Job) {
	q.jobs <- job
}

func (q *Queue) Dequeue() Job {
	return <-q.jobs
}
