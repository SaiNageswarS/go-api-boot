package bootUtils

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────
// GetEnvOrDefault
// ─────────────────────────────────────────────────────────────
func TestGetEnvOrDefault(t *testing.T) {
	const key = "BOOTUTILS_TEST"

	// 1️⃣ value present
	os.Setenv(key, "from_env")
	defer os.Unsetenv(key)

	if got := GetEnvOrDefault(key, "fallback"); got != "from_env" {
		t.Fatalf("got %q, want %q", got, "from_env")
	}

	// 2️⃣ value absent → fallback
	os.Unsetenv(key)
	if got := GetEnvOrDefault(key, "fallback"); got != "fallback" {
		t.Fatalf("got %q, want %q", got, "fallback")
	}
}

// ─────────────────────────────────────────────────────────────
// RetryWithExponentialBackoff
// ─────────────────────────────────────────────────────────────
func TestRetryWithExponentialBackoff_SucceedsAfterRetries(t *testing.T) {
	var tries int
	fn := func() error {
		tries++
		if tries < 3 {
			return errors.New("boom")
		}
		return nil
	}

	err := RetryWithExponentialBackoff(
		context.Background(),
		5,
		1*time.Millisecond, // fast test
		fn,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if tries != 3 {
		t.Fatalf("expected 3 attempts, got %d", tries)
	}
}

func TestRetryWithExponentialBackoff_ExhaustsAndFails(t *testing.T) {
	var tries int
	fn := func() error { tries++; return errors.New("always") }

	err := RetryWithExponentialBackoff(
		context.Background(),
		4,
		1*time.Millisecond,
		fn,
	)
	if err == nil || err.Error() != "all attempts failed" {
		t.Fatalf("expected final failure, got %v", err)
	}
	if tries != 4 {
		t.Fatalf("expected 4 attempts, got %d", tries)
	}
}

func TestRetryWithExponentialBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	fn := func() error { return errors.New("never called") }

	start := time.Now()
	err := RetryWithExponentialBackoff(ctx, 10, 10*time.Millisecond, fn)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if time.Since(start) > 20*time.Millisecond {
		t.Fatalf("function did not return promptly after cancellation")
	}
}
