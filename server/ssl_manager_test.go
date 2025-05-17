package server

import (
	"context"
	"runtime"
	"testing"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

func TestNewSSLManager_WithDomain(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")

	mgr := NewSSLManager(tempCache(t))

	if got, want := mgr.domain, "example.com"; got != want {
		t.Fatalf("mgr.domain = %q, want %q", got, want)
	}

	// quick sanity: HostPolicy must approve our domain
	if err := mgr.certManager.HostPolicy(context.Background(), "example.com"); err != nil {
		t.Fatalf("HostPolicy rejected domain: %v", err)
	}
}

/*
────────────────────────────────────────────────────────────────────────────────
RunAcmeChallengeListener test

The function should return immediately (it spawns a goroutine).
We don’t assert on the ListenAndServe outcome – it will fail quickly on
port 80 in most test environments, which is acceptable.
*/
func TestRunAcmeChallengeListener_NonBlocking(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Test uses :http which is unsupported on Windows")
	}

	t.Setenv("DOMAIN", "example.com")
	mgr := NewSSLManager(tempCache(t))

	done := make(chan struct{})
	go func() {
		mgr.RunAcmeChallengeListener()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("RunAcmeChallengeListener blocked the caller")
	}
}

func tempCache(t *testing.T) autocert.Cache {
	t.Helper()
	return autocert.DirCache(t.TempDir())
}
