package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

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
