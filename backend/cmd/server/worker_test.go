package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

type fakeInputDownloader struct {
	content []byte
	err     error
}

type fakeOutputUploader struct {
	key         string
	contentType string
	content     []byte
	err         error
}

func (f *fakeOutputUploader) UploadOutputObject(_ context.Context, key string, body io.Reader, contentType string) error {
	if f.err != nil {
		return f.err
	}
	f.key = key
	f.contentType = contentType
	var err error
	f.content, err = io.ReadAll(body)
	return err
}

func (f fakeInputDownloader) DownloadInputObject(context.Context, string) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(bytes.NewReader(f.content)), nil
}

func TestDownloadJobInputCreatesExtensionAndContents(t *testing.T) {
	path, err := downloadJobInput(context.Background(), fakeInputDownloader{content: []byte("a,b\n1,2\n")}, "input/students.csv")
	if err != nil {
		t.Fatalf("downloadJobInput returned error: %v", err)
	}
	defer os.Remove(path)

	if filepath.Ext(path) != ".csv" {
		t.Fatalf("temporary input extension = %q, want .csv", filepath.Ext(path))
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temporary input: %v", err)
	}
	if string(content) != "a,b\n1,2\n" {
		t.Fatalf("temporary input content = %q", content)
	}
}

func TestExecuteWithInputCleansTemporaryFileAfterSuccessAndFailure(t *testing.T) {
	worker := &Worker{ID: "test-worker", downloader: fakeInputDownloader{content: []byte("ERROR\n")}}
	var successPath string
	if _, err := worker.executeWithInput(context.Background(), Job{Type: "log_analysis", InputKey: "input/test.log"}, func(job Job) (int, error) {
		successPath = job.InputPath
		return analyzeLog(job.InputPath)
	}); err != nil {
		t.Fatalf("successful execution returned error: %v", err)
	}
	if _, err := os.Stat(successPath); !os.IsNotExist(err) {
		t.Fatalf("temporary file after success exists or stat failed: %v", err)
	}

	var failurePath string
	if _, err := worker.executeWithInput(context.Background(), Job{Type: "log_analysis", InputKey: "input/test.log"}, func(job Job) (int, error) {
		failurePath = job.InputPath
		return 0, errors.New("processing failed")
	}); err == nil {
		t.Fatal("failed execution returned nil error")
	}
	if _, err := os.Stat(failurePath); !os.IsNotExist(err) {
		t.Fatalf("temporary file after failure exists or stat failed: %v", err)
	}
}

func TestDownloadJobInputFailure(t *testing.T) {
	_, err := downloadJobInput(context.Background(), fakeInputDownloader{err: errors.New("S3 unavailable")}, "input/test.log")
	if err == nil || !strings.Contains(err.Error(), "download input object") {
		t.Fatalf("download failure = %v", err)
	}
}

func TestExecuteWithInputLeavesNonS3JobsUnchanged(t *testing.T) {
	worker := &Worker{}
	job := Job{Type: "addition", A: 2, B: 3}
	result, err := worker.executeWithInput(context.Background(), job, executeJob)
	if err != nil || result != 5 {
		t.Fatalf("non-S3 execution result=%d err=%v", result, err)
	}
}

func TestUploadJobOutputUsesDeterministicKeyAndContent(t *testing.T) {
	uploader := &fakeOutputUploader{}
	worker := &Worker{ID: "test-worker", uploader: uploader}
	job := Job{ID: "job-123", Type: "csv_processing"}

	key, err := worker.uploadJobOutput(context.Background(), job, 7)
	if err != nil {
		t.Fatalf("uploadJobOutput returned error: %v", err)
	}
	if key != "output/job-123/result.csv" || uploader.key != key {
		t.Fatalf("uploaded key = %q, want %q", uploader.key, "output/job-123/result.csv")
	}
	if uploader.contentType != "text/csv" || string(uploader.content) != "result\n7\n" {
		t.Fatalf("uploaded output = type %q content %q", uploader.contentType, uploader.content)
	}

	secondKey, err := worker.uploadJobOutput(context.Background(), job, 7)
	if err != nil || secondKey != key {
		t.Fatalf("repeat upload key=%q err=%v, want deterministic key %q", secondKey, err, key)
	}
}

func TestUploadJobOutputFailureIsReturned(t *testing.T) {
	worker := &Worker{uploader: &fakeOutputUploader{err: errors.New("S3 unavailable")}}
	_, err := worker.uploadJobOutput(context.Background(), Job{ID: "job-fail", Type: "log_analysis"}, 2)
	if err == nil || !strings.Contains(err.Error(), "upload job output") {
		t.Fatalf("upload failure = %v", err)
	}
}

func TestNonS3JobDoesNotUploadOutput(t *testing.T) {
	uploader := &fakeOutputUploader{}
	worker := &Worker{uploader: uploader}
	key, err := worker.uploadJobOutput(context.Background(), Job{ID: "job-number", Type: "addition"}, 5)
	if err != nil || key != "" || uploader.key != "" {
		t.Fatalf("non-S3 output key=%q uploader key=%q err=%v", key, uploader.key, err)
	}
}

func TestWorkerUploadFailureDoesNotCompleteJob(t *testing.T) {
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
	jobID := fmt.Sprintf("test-upload-failure-%d", time.Now().UnixNano())
	defer func() {
		if _, err := db.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, jobID); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	if err := store.CreateContext(ctx, Job{ID: jobID, Type: "log_analysis", InputKey: "input/test.log"}); err != nil {
		t.Fatalf("CreateContext failed: %v", err)
	}
	claimed, ok, err := store.ClaimNext(ctx, "upload-worker", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("ClaimNext: claimed=%v err=%v", ok, err)
	}
	worker := &Worker{
		ID:         "upload-worker",
		store:      store,
		downloader: fakeInputDownloader{content: []byte("ERROR\n")},
		uploader:   &fakeOutputUploader{err: errors.New("S3 unavailable")},
	}
	worker.process(ctx, claimed)

	job, found, err := store.GetContext(ctx, jobID)
	if err != nil || !found {
		t.Fatalf("GetContext: found=%v err=%v", found, err)
	}
	if job.Status != "queued" || job.Retries != 1 || job.OutputKey != "" {
		t.Fatalf("upload failure incorrectly completed job: %+v", job)
	}
}

func TestRunJobSafelyConvertsPanicToError(t *testing.T) {
	result, err := runJobSafely(func() (int, error) {
		panic("boom")
	})

	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected panic error containing %q, got %v", "boom", err)
	}
	if result != 0 {
		t.Fatalf("expected zero result after panic, got %d", result)
	}
}

func TestWorkerShutdownDuringJobDoesNotCompleteAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	worker := &Worker{ID: "controlled", execute: func(Job) (int, error) {
		close(started)
		<-release
		return 42, nil
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.process(ctx, Job{ID: "shutdown-job", Attempt: 1})
	}()
	<-started
	cancel()
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not exit after the controlled job returned")
	}
}

func TestWaitForBackgroundWaitsUntilAllGoroutinesExit(t *testing.T) {
	var background sync.WaitGroup
	background.Add(1)
	release := make(chan struct{})
	go func() {
		defer background.Done()
		<-release
	}()
	deadline, cancelDeadline := context.WithTimeout(context.Background(), time.Second)
	defer cancelDeadline()
	if waitForBackground(&background, deadline.Done()) {
		t.Fatal("wait returned before background goroutine exited")
	}
	close(release)
	deadline, cancelDeadline = context.WithTimeout(context.Background(), time.Second)
	defer cancelDeadline()
	if !waitForBackground(&background, deadline.Done()) {
		t.Fatal("wait did not return after background goroutine exited")
	}
}
