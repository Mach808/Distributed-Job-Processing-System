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
	InputKey   string `json:"input_key,omitempty"`
	OutputKey  string `json:"output_key,omitempty"`
	Query      string `json:"query,omitempty"`
}
