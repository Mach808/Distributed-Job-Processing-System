package main

import "time"

type Job struct {
	ID         string
	Type       string
	A          int
	B          int
	Status     string
	Result     int
	Retries    int
	LeaseUntil time.Time
	Attempt    int
	WorkerID   string
	InputPath  string `json:"-"`
	Query      string `json:"Query,omitempty"`
}
