package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func generateJobID() string { return fmt.Sprintf("job-%d", time.Now().UnixNano()) }

type JobRequest struct {
	Type     string `json:"type"`
	A        int    `json:"a"`
	B        int    `json:"b"`
	InputKey string `json:"input_key"`
	Query    string `json:"query"`
}

var supportedJobTypes = map[string]bool{
	"addition": true, "multiplication": true, "subtraction": true,
	"division": true, "fibonacci": true, "prime_count": true, "retry_demo": true,
	"image_processing": true, "log_analysis": true, "csv_processing": true,
}

type Handler struct {
	store *JobStore
	s3    S3Service
}

type downloadResponse struct {
	DownloadURL string `json:"download_url"`
	ExpiresIn   int64  `json:"expires_in"`
}

func NewHandler(store *JobStore, s3Client S3Service) *Handler {
	return &Handler{store: store, s3: s3Client}
}

func (h *Handler) ListS3Files(w http.ResponseWriter, r *http.Request) {
	if h.s3 == nil {
		http.Error(w, "S3 is not configured", http.StatusServiceUnavailable)
		return
	}

	files, err := h.s3.ListInputFiles(r.Context())
	if err != nil {
		log.Printf("list S3 input files: %v", err)
		http.Error(w, "Unable to list S3 input files", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string][]S3File{"files": files}); err != nil {
		log.Printf("write S3 file list response: %v", err)
	}
}

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
	if isFileProcessingJob(request.Type) {
		if h.s3 == nil {
			http.Error(w, "S3 is not configured", http.StatusServiceUnavailable)
			return
		}
		if err := verifyInputObject(r.Context(), h.s3, request.InputKey); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	request.Query = strings.TrimSpace(request.Query)
	job := Job{ID: generateJobID(), Type: request.Type, A: request.A, B: request.B, InputKey: request.InputKey, Query: request.Query, Status: "queued"}
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
		if err := validateInputKey(request.InputKey); err != nil {
			return fmt.Errorf("%s: %w", request.Type, err)
		}
		if request.Type == "csv_processing" && strings.TrimSpace(request.Query) == "" {
			return fmt.Errorf("csv_processing requires query")
		}
	}
	return nil
}

func isFileProcessingJob(jobType string) bool {
	return jobType == "csv_processing" || jobType == "log_analysis" || jobType == "image_processing"
}

func validateInputKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("requires input_key")
	}
	if key != strings.TrimSpace(key) {
		return fmt.Errorf("input_key must not have leading or trailing whitespace")
	}
	if strings.ContainsAny(key, "\\\x00") || !strings.HasPrefix(key, "input/") || key == "input/" {
		return fmt.Errorf("input_key must be a valid key under input/")
	}
	parts := strings.Split(key, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("input_key must not contain empty or traversal path segments")
		}
	}
	return nil
}

func verifyInputObject(ctx context.Context, store S3ObjectStore, key string) error {
	if err := validateInputKey(key); err != nil {
		return err
	}
	if err := store.HeadInputObject(ctx, key); err != nil {
		return fmt.Errorf("input object does not exist")
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

func (h *Handler) DownloadJob(w http.ResponseWriter, r *http.Request) {
	job, ok, err := h.store.GetContext(r.Context(), r.PathValue("id"))
	if err != nil {
		log.Printf("get job for download: %v", err)
		http.Error(w, "Failed to retrieve job", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}
	if job.Status != "completed" {
		http.Error(w, "Job is not completed", http.StatusConflict)
		return
	}
	if err := validateOutputKey(job.OutputKey); err != nil {
		http.Error(w, "Job output is not available", http.StatusNotFound)
		return
	}
	presigner, ok := h.s3.(S3OutputPresigner)
	if !ok {
		http.Error(w, "S3 is not configured", http.StatusServiceUnavailable)
		return
	}
	url, expiry, err := presigner.PresignOutputGet(r.Context(), job.OutputKey)
	if err != nil {
		log.Printf("presign output for job %s: %v", job.ID, err)
		http.Error(w, "Unable to create download URL", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(downloadResponse{DownloadURL: url, ExpiresIn: int64(expiry / time.Second)}); err != nil {
		log.Printf("write download response: %v", err)
	}
}

func validateOutputKey(key string) error {
	if strings.TrimSpace(key) == "" || key != strings.TrimSpace(key) || !strings.HasPrefix(key, "output/") || key == "output/" {
		return fmt.Errorf("output key must be under output/")
	}
	if strings.ContainsAny(key, "\\\x00") {
		return fmt.Errorf("output key contains invalid characters")
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("output key contains invalid path segments")
		}
	}
	return nil
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
