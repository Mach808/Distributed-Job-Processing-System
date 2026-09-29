package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

var jobCounter uint64
var jobCounterMu sync.Mutex

func generateJobID() string {
	jobCounterMu.Lock()
	defer jobCounterMu.Unlock()

	jobCounter++

	return fmt.Sprintf("job-%d", jobCounter)
}

type JobRequest struct {
	Type string `json:"type"`
	A    int    `json:"a"`
	B    int    `json:"b"`
}

type Handler struct {
	queue *Queue
	store *JobStore
}

func NewHandler(queue *Queue, store *JobStore) *Handler {
	return &Handler{
		queue: queue,
		store: store,
	}
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var request JobRequest

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	fmt.Println("Received job:", request.Type)

	job := Job{
		ID:     generateJobID(),
		Type:   request.Type,
		A:      request.A,
		B:      request.B,
		Status: "queued",
	}

	h.store.Set(job)

	h.queue.Enqueue(job)

	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(job)
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	job, ok := h.store.Get(id)

	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(job)
}
