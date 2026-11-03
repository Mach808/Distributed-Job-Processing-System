package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	leaseDuration = 30 * time.Second
	pollInterval  = 500 * time.Millisecond
	maxRetries    = 3
)

type Worker struct {
	ID         string
	store      *JobStore
	execute    func(Job) (int, error)
	downloader S3InputDownloader
	uploader   S3OutputUploader
}

func NewWorker(id string, store *JobStore, downloaders ...S3InputDownloader) *Worker {
	worker := &Worker{ID: id, store: store, execute: executeJob}
	if len(downloaders) > 0 {
		worker.downloader = downloaders[0]
		if uploader, ok := downloaders[0].(S3OutputUploader); ok {
			worker.uploader = uploader
		}
	}
	return worker
}

// Start continuously claims jobs from PostgreSQL. The database is the only queue;
// workers never depend on process-local job state.
func (w *Worker) Start(ctx context.Context) {
	log.Printf("worker %s started", w.ID)
	for {
		if ctx.Err() != nil {
			return
		}

		job, claimed, err := w.store.ClaimNext(ctx, w.ID, leaseDuration)
		if err != nil {
			log.Printf("worker %s: claim job: %v", w.ID, err)
			if !sleepContext(ctx, pollInterval) {
				return
			}
			continue
		}
		if !claimed {
			if !sleepContext(ctx, pollInterval) {
				return
			}
			continue
		}

		w.process(ctx, job)
	}
}

func (w *Worker) process(parent context.Context, job Job) {
	ctx, cancel := context.WithCancel(parent)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(leaseDuration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ok, err := w.store.RenewLeaseContext(ctx, job.ID, w.ID, job.Attempt, leaseDuration)
				if err != nil {
					log.Printf("worker %s: renew lease for %s: %v", w.ID, job.ID, err)
					cancel()
					return
				}
				if !ok {
					log.Printf("worker %s lost ownership of job %s attempt %d", w.ID, job.ID, job.Attempt)
					cancel()
					return
				}
			}
		}
	}()

	execute := w.execute
	if execute == nil {
		execute = executeJob
	}
	outputKey := ""
	result, err := runJobSafely(func() (int, error) {
		result, err := w.executeWithInput(ctx, job, execute)
		if err != nil {
			return 0, err
		}
		outputKey, err = w.uploadJobOutput(ctx, job, result)
		return result, err
	})
	cancel()
	<-heartbeatDone
	if parent.Err() != nil {
		return
	}
	if err != nil {
		failed, failErr := w.store.FailAttempt(parent, job.ID, w.ID, job.Attempt, maxRetries)
		if failErr != nil {
			log.Printf("worker %s: record failure for %s: %v; lease recovery will handle it", w.ID, job.ID, failErr)
			return
		}
		if failed {
			log.Printf("worker %s: job %s failed: %v", w.ID, job.ID, err)
		}
		return
	}

	completed, completeErr := w.store.CompleteContext(parent, job.ID, w.ID, job.Attempt, result, outputKey)
	if completeErr != nil {
		log.Printf("worker %s: complete job %s: %v; lease recovery will handle it", w.ID, job.ID, completeErr)
		return
	}
	if completed {
		log.Printf("worker %s completed job %s result=%d", w.ID, job.ID, result)
	}
}

func (w *Worker) uploadJobOutput(ctx context.Context, job Job, result int) (string, error) {
	if !isFileProcessingJob(job.Type) {
		return "", nil
	}
	if w.uploader == nil {
		return "", fmt.Errorf("S3 output uploader is not configured")
	}

	path, key, contentType, err := createJobOutput(job, result)
	if err != nil {
		return "", err
	}
	defer func() {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Printf("worker %s: remove temporary output: %v", w.ID, removeErr)
		}
	}()

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open temporary output: %w", err)
	}
	defer file.Close()
	if err := w.uploader.UploadOutputObject(ctx, key, file, contentType); err != nil {
		return "", fmt.Errorf("upload job output: %w", err)
	}
	return key, nil
}

func createJobOutput(job Job, result int) (string, string, string, error) {
	var extension, content string
	contentType := "text/plain"
	switch job.Type {
	case "csv_processing":
		extension = ".csv"
		contentType = "text/csv"
		content = fmt.Sprintf("result\n%d\n", result)
	case "log_analysis":
		extension = ".log"
		content = fmt.Sprintf("result=%d\n", result)
	case "image_processing":
		extension = ".txt"
		content = fmt.Sprintf("result=%d\n", result)
	default:
		return "", "", "", fmt.Errorf("job type %q does not produce an S3 output", job.Type)
	}

	file, err := os.CreateTemp("", "djs-job-output-*"+extension)
	if err != nil {
		return "", "", "", fmt.Errorf("create temporary output file: %w", err)
	}
	path := file.Name()
	removeOnError := true
	defer func() {
		if removeOnError {
			_ = os.Remove(path)
		}
	}()
	if _, err := io.WriteString(file, content); err != nil {
		_ = file.Close()
		return "", "", "", fmt.Errorf("write temporary output file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", "", "", fmt.Errorf("close temporary output file: %w", err)
	}
	removeOnError = false
	return path, fmt.Sprintf("output/%s/result%s", job.ID, extension), contentType, nil
}

func (w *Worker) executeWithInput(ctx context.Context, job Job, execute func(Job) (int, error)) (int, error) {
	if !isFileProcessingJob(job.Type) {
		return execute(job)
	}
	if err := validateInputKey(job.InputKey); err != nil {
		return 0, fmt.Errorf("invalid input key: %w", err)
	}
	if w.downloader == nil {
		return 0, fmt.Errorf("S3 input downloader is not configured")
	}

	localPath, err := downloadJobInput(ctx, w.downloader, job.InputKey)
	if err != nil {
		return 0, err
	}
	defer func() {
		if removeErr := os.Remove(localPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Printf("worker %s: remove temporary input: %v", w.ID, removeErr)
		}
	}()

	job.InputPath = localPath
	return execute(job)
}

func downloadJobInput(ctx context.Context, downloader S3InputDownloader, inputKey string) (string, error) {
	body, err := downloader.DownloadInputObject(ctx, inputKey)
	if err != nil {
		return "", fmt.Errorf("download input object: %w", err)
	}
	defer body.Close()

	extension := filepath.Ext(filepath.Base(inputKey))
	file, err := os.CreateTemp("", "djs-job-input-*"+extension)
	if err != nil {
		return "", fmt.Errorf("create temporary input file: %w", err)
	}
	path := file.Name()
	removeOnError := true
	defer func() {
		if removeOnError {
			_ = os.Remove(path)
		}
	}()

	if _, err := io.Copy(file, body); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write temporary input file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close temporary input file: %w", err)
	}
	removeOnError = false
	return path, nil
}

// runJobSafely contains panics raised while executing one job and turns them
// into ordinary attempt failures so the caller can apply the retry policy.
func runJobSafely(run func() (int, error)) (result int, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = 0
			err = fmt.Errorf("job processing panicked: %v", recovered)
		}
	}()
	return run()
}

func executeJob(job Job) (int, error) {
	switch job.Type {
	case "addition":
		return job.A + job.B, nil
	case "multiplication":
		return job.A * job.B, nil
	case "subtraction":
		return job.A - job.B, nil
	case "division":
		if job.B == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return job.A / job.B, nil
	case "fibonacci":
		if job.A < 0 || job.A > 46 {
			return 0, fmt.Errorf("fibonacci input must be between 0 and 46")
		}
		previous, current := 0, 1
		for i := 0; i < job.A; i++ {
			previous, current = current, previous+current
		}
		return previous, nil
	case "prime_count":
		if job.A < 2 || job.A > 10000000 {
			return 0, fmt.Errorf("prime_count input must be between 2 and 10000000")
		}
		return countPrimes(job.A), nil
	case "retry_demo":
		if job.A < 0 || job.A >= maxRetries {
			return 0, fmt.Errorf("retry_demo failure count must be between 0 and %d", maxRetries-1)
		}
		if job.Retries < job.A {
			return 0, fmt.Errorf("demo failure %d of %d", job.Retries+1, job.A)
		}
		return job.B, nil
	case "image_processing":
		return processImage(job.InputPath)
	case "log_analysis":
		return analyzeLog(job.InputPath)
	case "csv_processing":
		return analyzeCSV(job.InputPath, job.Query)
	default:
		return 0, fmt.Errorf("unsupported job type %q", job.Type)
	}
}

func processImage(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open image: %w", err)
	}
	defer file.Close()
	if _, _, err := image.DecodeConfig(file); err != nil {
		return 0, fmt.Errorf("decode image: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat image: %w", err)
	}
	return int(info.Size()), nil
}

func analyzeLog(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open log: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		if strings.Contains(strings.ToUpper(scanner.Text()), "ERROR") {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read log: %w", err)
	}
	return count, nil
}

func analyzeCSV(path, query string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open csv: %w", err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	matches := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read csv: %w", err)
		}
		for _, value := range row {
			if strings.TrimSpace(value) == query {
				matches++
			}
		}
	}
	return matches, nil
}

func countPrimes(limit int) int {
	composite := make([]bool, limit+1)
	count := 0
	for candidate := 2; candidate <= limit; candidate++ {
		if composite[candidate] {
			continue
		}
		count++
		if candidate <= limit/candidate {
			for multiple := candidate * candidate; multiple <= limit; multiple += candidate {
				composite[multiple] = true
			}
		}
	}
	return count
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
