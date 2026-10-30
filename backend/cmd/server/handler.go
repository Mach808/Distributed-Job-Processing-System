package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func generateJobID() string { return fmt.Sprintf("job-%d", time.Now().UnixNano()) }

type JobRequest struct {
	Type string `json:"type"`
	A    int    `json:"a"`
	B    int    `json:"b"`
	InputPath string `json:"input_path"`
	Query string `json:"query"`
}

var supportedJobTypes = map[string]bool{
	"addition": true, "multiplication": true, "subtraction": true,
	"division": true, "fibonacci": true, "prime_count": true, "retry_demo": true,
	"image_processing": true, "log_analysis": true, "csv_processing": true,
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
	if request.Type == "image_processing" || request.Type == "log_analysis" || request.Type == "csv_processing" {
		cleanPath, err := localInputPath(request.InputPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		request.InputPath = cleanPath
	}
	request.Query = strings.TrimSpace(request.Query)
	job := Job{ID: generateJobID(), Type: request.Type, A: request.A, B: request.B, InputPath: request.InputPath, Query: request.Query, Status: "queued"}
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
	case "image_processing", "log_analysis", "csv_processing":
		if strings.TrimSpace(request.InputPath) == "" {
			return fmt.Errorf("%s requires input_path", request.Type)
		}
		if request.Type == "csv_processing" && strings.TrimSpace(request.Query) == "" {
			return fmt.Errorf("csv_processing requires query")
		}
	}
	return nil
}

func localInputPath(input string) (string, error) {
	root := os.Getenv("JOB_DATA_DIR")
	if root == "" {
		root = "data/jobs"
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("invalid job data directory")
	}
	clean := filepath.Clean(input)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("input_path must be a relative path inside %s", root)
	}
	full := filepath.Join(root, clean)
	if !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", fmt.Errorf("input_path must stay inside %s", root)
	}
	if _, err := os.Stat(full); err != nil {
		return "", fmt.Errorf("input file is not available: %w", err)
	}
	return full, nil
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

func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.store.ListRecentContext(r.Context(), 100)
	if err != nil {
		log.Printf("list jobs: %v", err)
		http.Error(w, "Failed to list jobs", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(jobs); err != nil {
		log.Printf("write list response: %v", err)
	}
}
