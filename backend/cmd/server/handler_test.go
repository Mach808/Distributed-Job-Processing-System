package main

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

type fakeS3ObjectStore struct {
	err     error
	seenKey string
	url     string
	expires time.Duration
}

func (f *fakeS3ObjectStore) ListInputFiles(context.Context) ([]S3File, error) {
	return nil, nil
}

func (f *fakeS3ObjectStore) HeadInputObject(_ context.Context, key string) error {
	f.seenKey = key
	return f.err
}

func (f *fakeS3ObjectStore) PresignOutputGet(_ context.Context, key string) (string, time.Duration, error) {
	f.seenKey = key
	return f.url, f.expires, f.err
}

func TestValidateInputKey(t *testing.T) {
	valid := []string{"input/students.csv", "input/nested/run-1.log"}
	for _, key := range valid {
		if err := validateInputKey(key); err != nil {
			t.Errorf("validateInputKey(%q) returned error: %v", key, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"output/students.csv",
		"/input/students.csv",
		"input/../students.csv",
		"input/nested/../../students.csv",
		`input\\students.csv`,
		"input//students.csv",
	}
	for _, key := range invalid {
		if err := validateInputKey(key); err == nil {
			t.Errorf("validateInputKey(%q) accepted invalid key", key)
		}
	}
}

func TestVerifyInputObject(t *testing.T) {
	store := &fakeS3ObjectStore{}
	if err := verifyInputObject(context.Background(), store, "input/students.csv"); err != nil {
		t.Fatalf("verifyInputObject returned error: %v", err)
	}
	if store.seenKey != "input/students.csv" {
		t.Fatalf("HeadObject key = %q, want %q", store.seenKey, "input/students.csv")
	}

	store.err = errors.New("not found")
	if err := verifyInputObject(context.Background(), store, "input/missing.csv"); err == nil {
		t.Fatal("verifyInputObject accepted a missing object")
	}
}

func TestNonFileJobDoesNotRequireInputKey(t *testing.T) {
	for _, jobType := range []string{"addition", "multiplication"} {
		if err := validateJobRequest(JobRequest{Type: jobType, A: 2, B: 3}); err != nil {
			t.Errorf("validateJobRequest(%q) returned error: %v", jobType, err)
		}
	}
}

func TestValidateOutputKey(t *testing.T) {
	for _, key := range []string{"output/job-1/result.csv", "output/nested/result.txt"} {
		if err := validateOutputKey(key); err != nil {
			t.Errorf("validateOutputKey(%q) returned error: %v", key, err)
		}
	}
	for _, key := range []string{"", "input/result.csv", "output/../result.csv", `output\\result.csv`, "output//result.csv"} {
		if err := validateOutputKey(key); err == nil {
			t.Errorf("validateOutputKey(%q) accepted invalid key", key)
		}
	}
}

func TestConfiguredPresignedURLExpiry(t *testing.T) {
	old := os.Getenv(presignedURLExpiryEnv)
	defer os.Setenv(presignedURLExpiryEnv, old)

	os.Unsetenv(presignedURLExpiryEnv)
	expiry, err := configuredPresignedURLExpiry()
	if err != nil || expiry != defaultPresignedURLExpiry {
		t.Fatalf("default expiry=%v err=%v", expiry, err)
	}

	os.Setenv(presignedURLExpiryEnv, "120")
	expiry, err = configuredPresignedURLExpiry()
	if err != nil || expiry != 120*time.Second {
		t.Fatalf("configured expiry=%v err=%v", expiry, err)
	}

	os.Setenv(presignedURLExpiryEnv, "0")
	if _, err := configuredPresignedURLExpiry(); err == nil {
		t.Fatal("zero expiry was accepted")
	}
}

func TestDownloadJobEndpointUsesStoredOutputKey(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}
	db, err := connectDB()
	if err != nil {
		t.Fatalf("connectDB failed: %v", err)
	}
	defer db.Close()

	store := NewJobStore(db)
	ctx := context.Background()
	jobID := fmt.Sprintf("test-download-%d", time.Now().UnixNano())
	defer func() {
		if _, err := db.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, jobID); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()
	outputKey := "output/" + jobID + "/result.csv"
	if err := store.CreateContext(ctx, Job{ID: jobID, Type: "csv_processing"}); err != nil {
		t.Fatalf("CreateContext failed: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE jobs SET status = 'completed', output_key = $2 WHERE id = $1`, jobID, outputKey); err != nil {
		t.Fatalf("mark completed failed: %v", err)
	}

	s3 := &fakeS3ObjectStore{url: "https://example.test/presigned", expires: 15 * time.Minute}
	handler := NewHandler(store, s3)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/jobs/ignored/download", nil)
	request.SetPathValue("id", jobID)
	handler.DownloadJob(recorder, request)

	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), s3.url) {
		t.Fatalf("download response status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if s3.seenKey != outputKey {
		t.Fatalf("presigned key=%q, want stored key %q", s3.seenKey, outputKey)
	}
	if _, err := db.Exec(ctx, `UPDATE jobs SET status = 'queued' WHERE id = $1`, jobID); err != nil {
		t.Fatalf("reset job status failed: %v", err)
	}
	recorder = httptest.NewRecorder()
	handler.DownloadJob(recorder, request)
	if recorder.Code != 409 {
		t.Fatalf("incomplete job status=%d, want 409", recorder.Code)
	}
}
