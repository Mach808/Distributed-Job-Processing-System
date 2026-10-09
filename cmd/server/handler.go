package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func generateJobID() string { return fmt.Sprintf("job-%d", time.Now().UnixNano()) }

type JobRequest struct {
	Type string `json:"type"`
	A    int    `json:"a"`
	B    int    `json:"b"`
}

var supportedJobTypes = map[string]bool{
	"addition": true, "multiplication": true, "subtraction": true,
	"division": true, "fibonacci": true, "prime_count": true, "retry_demo": true,
}

type Handler struct{ store *JobStore }

func NewHandler(store *JobStore) *Handler { return &Handler{store: store} }

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var request JobRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}
	request.Type = strings.TrimSpace(request.Type)
	if !supportedJobTypes[request.Type] {
		http.Error(w, "unsupported job type", http.StatusBadRequest)
		return
	}
	if err := validateJobRequest(request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	job := Job{ID: generateJobID(), Type: request.Type, A: request.A, B: request.B, Status: "queued"}
	if err := h.store.CreateContext(r.Context(), job); err != nil {
		log.Printf("create job: %v", err)
		http.Error(w, "Failed to create job", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(job); err != nil {
		log.Printf("write create response: %v", err)
	}
}

func validateJobRequest(request JobRequest) error {
	switch request.Type {
	case "division":
		if request.B == 0 {
			return fmt.Errorf("division requires a non-zero second number")
		}
	case "fibonacci":
		if request.A < 0 || request.A > 46 {
			return fmt.Errorf("fibonacci requires first number between 0 and 46")
		}
	case "prime_count":
		if request.A < 2 || request.A > 10000000 {
			return fmt.Errorf("prime_count requires first number between 2 and 10000000")
		}
	case "retry_demo":
		if request.A < 0 || request.A >= maxRetries {
			return fmt.Errorf("retry_demo requires first number between 0 and 2")
		}
	}
	return nil
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	job, ok, err := h.store.GetContext(r.Context(), r.PathValue("id"))
	if err != nil {
		log.Printf("get job: %v", err)
		http.Error(w, "Failed to retrieve job", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(job); err != nil {
		log.Printf("write get response: %v", err)
	}
}
