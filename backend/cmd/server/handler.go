package main

import (
	"crypto/subtle"
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

type Handler struct {
	store    *JobStore
	adminKey string
}

func NewHandler(store *JobStore, adminKey ...string) *Handler {
	configuredAdminKey := ""
	if len(adminKey) > 0 {
		configuredAdminKey = strings.TrimSpace(adminKey[0])
	}
	return &Handler{store: store, adminKey: configuredAdminKey}
}

type CreateAPIClientRequest struct {
	ClientID string `json:"client_id"`
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	ClientID string `json:"client_id"`
}

func (h *Handler) authorizeAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.adminKey == "" {
		http.NotFound(w, r)
		return false
	}
	providedAdminKey := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	if subtle.ConstantTimeCompare([]byte(providedAdminKey), []byte(h.adminKey)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (h *Handler) CreateAPIClient(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}

	var request CreateAPIClientRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}

	request.ClientID = strings.TrimSpace(request.ClientID)
	if request.ClientID == "" {
		http.Error(w, "client_id is required", http.StatusBadRequest)
		return
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		log.Printf("generate API key: %v", err)
		http.Error(w, "failed to generate API key", http.StatusInternalServerError)
		return
	}

	if err := h.store.CreateAPIClient(r.Context(), request.ClientID, hashAPIKey(apiKey)); err != nil {
		log.Printf("create API client: %v", err)
		http.Error(w, "failed to create API client", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"client_id": request.ClientID,
		"api_key":   apiKey,
	})
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}

	var request CreateUserRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}
	request.Username = strings.TrimSpace(request.Username)
	request.ClientID = strings.TrimSpace(request.ClientID)
	if request.Username == "" || request.ClientID == "" {
		http.Error(w, "username and client_id are required", http.StatusBadRequest)
		return
	}
	if len(request.Password) < 12 {
		http.Error(w, "password must be at least 12 characters", http.StatusBadRequest)
		return
	}

	passwordHash, err := hashPassword(request.Password)
	if err != nil {
		log.Printf("hash user password: %v", err)
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}
	if err := h.store.CreateUser(r.Context(), request.Username, passwordHash, request.ClientID); err != nil {
		log.Printf("create user: %v", err)
		http.Error(w, "failed to create user", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"username":  request.Username,
		"client_id": request.ClientID,
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}

	clientID, authenticated, err := h.store.AuthenticateUser(
		r.Context(), strings.TrimSpace(request.Username), request.Password,
	)
	if err != nil {
		log.Printf("login: %v", err)
		http.Error(w, "authentication service unavailable", http.StatusInternalServerError)
		return
	}
	if !authenticated {
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}

	token, err := randomToken()
	if err != nil {
		log.Printf("generate session token: %v", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	const sessionDuration = 24 * time.Hour
	if err := h.store.CreateSession(r.Context(), hashSessionToken(token), clientID, time.Now().Add(sessionDuration)); err != nil {
		log.Printf("create session: %v", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "job_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"client_id": clientID})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("job_session"); err == nil && cookie.Value != "" {
		if err := h.store.DeleteSession(r.Context(), hashSessionToken(cookie.Value)); err != nil {
			log.Printf("delete session: %v", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "job_session", Value: "", Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) authenticateRequest(r *http.Request) (string, bool, error) {
	if cookie, err := r.Cookie("job_session"); err == nil && cookie.Value != "" {
		clientID, authenticated, sessionErr := h.store.AuthenticateSession(r.Context(), hashSessionToken(cookie.Value))
		if sessionErr != nil || authenticated {
			return clientID, authenticated, sessionErr
		}
	}
	apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if apiKey == "" {
		return "", false, nil
	}
	return h.store.AuthenticateAPIKey(r.Context(), apiKey)
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	_, authenticated, err := h.authenticateRequest(r)
	if err != nil {
		log.Printf("authenticate request: %v", err)
		http.Error(w, "authentication service unavailable", http.StatusInternalServerError)
		return
	}
	if !authenticated {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

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

	if request.Type == "image_processing" ||
		request.Type == "log_analysis" ||
		request.Type == "csv_processing" {
		cleanPath, err := localInputPath(request.InputPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		request.InputPath = cleanPath
	}

	request.Query = strings.TrimSpace(request.Query)

	job := Job{
		ID:        generateJobID(),
		Type:      request.Type,
		A:         request.A,
		B:         request.B,
		InputPath: request.InputPath,
		Query:     request.Query,
		Status:    "queued",
	}

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
	if _, authenticated, err := h.authenticateRequest(r); err != nil {
		http.Error(w, "authentication service unavailable", http.StatusInternalServerError)
		return
	} else if !authenticated {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
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
	if _, authenticated, err := h.authenticateRequest(r); err != nil {
		http.Error(w, "authentication service unavailable", http.StatusInternalServerError)
		return
	} else if !authenticated {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
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
