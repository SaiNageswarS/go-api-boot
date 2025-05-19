package server

import (
	"context"
	"runtime"
	"testing"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// ─────────────────────────────────────────────────────────────
// NewSSLManager – constructor sanity check
// ─────────────────────────────────────────────────────────────
func TestNewSSLManager_WithDomain(t *testing.T) {
	mgr := NewSSLManager("example.com", tempCache(t))

	if got, want := mgr.domain, "example.com"; got != want {
		t.Fatalf("mgr.domain = %q, want %q", got, want)
	}

	// HostPolicy must approve the given domain.
	if err := mgr.certManager.HostPolicy(context.Background(), "example.com"); err != nil {
		t.Fatalf("HostPolicy rejected domain: %v", err)
	}
}

/*
────────────────────────────────────────────────────────────────────────────────
Run(ctx) must respect context cancellation promptly.

We don’t assert on the ListenAndServe outcome – it is expected to fail on
port 80 in most CI environments, and that is fine as long as Run() returns.
*/
func TestRun_ContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ACME :http listener is unsupported on Windows CI")
	}

	mgr := NewSSLManager("example.com", tempCache(t))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		_ = mgr.Run(ctx) // ignore error (port 80 likely unavailable)
		close(done)
	}()

	// Give the goroutine a brief moment to start.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Run(ctx) did not return promptly after cancellation")
	}
}

// helper: temp directory cache
func tempCache(t *testing.T) autocert.Cache {
	t.Helper()
	return autocert.DirCache(t.TempDir())
}
