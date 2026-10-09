package main

import (
	"strings"
	"testing"
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
