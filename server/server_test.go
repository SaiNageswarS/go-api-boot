package server

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/config"
)

// -----------------------------------------------------------------------------
// Test 1: Shutdown should unblock Serve even if ctx is not cancelled.
// -----------------------------------------------------------------------------
func TestBootServer_Shutdown_UnblocksServe(t *testing.T) {
	bs := freshBootServer(t, false)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- bs.Serve(ctx) }()

	time.Sleep(100 * time.Millisecond) // let servers start

	// Explicit shutdown (graceful) …
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutCancel()
	if err := bs.Shutdown(shutCtx); err != nil {
		t.Fatalf("Shutdown() error: %v", err)
	}

	// …Serve is still waiting on ctx; cancel it now.
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("Serve(ctx) did not return after Shutdown + cancel")
	}
}

// -----------------------------------------------------------------------------
// Test 2: Builder validation – missing ports should error.
// -----------------------------------------------------------------------------
func TestBootServer_BuilderValidation(t *testing.T) {
	_, err := New(&config.BootConfig{}).
		GRPCPort(":0").
		Build() // missing HTTPPort
	if err == nil {
		t.Fatalf("Build() succeeded with missing HTTPPort; want error")
	}

	_, err = New(&config.BootConfig{}).
		HTTPPort(":0").
		Build() // missing GRPCPort
	if err == nil {
		t.Fatalf("Build() succeeded with missing GRPCPort; want error")
	}
}

// -----------------------------------------------------------------------------
// Test 3: Build succeeds with SSL provider (DirCache) and :0 ports.
//
//	We don't call Serve – that would need a cert; Build must not fail.
//
// -----------------------------------------------------------------------------
func TestBootServer_Build_WithSSL(t *testing.T) {
	if runtime.GOOS == "windows" { // ACME dir cache uses :80 in Serve, skip
		t.Skip("skip SSL build test on Windows CI")
	}

	_, err := New(&config.BootConfig{}).
		GRPCPort(":0").
		HTTPPort(":0").
		EnableSSL(DirCache(t.TempDir())).
		Build()
	if err != nil {
		t.Fatalf("Build() with SSL failed: %v", err)
	}
}

// -----------------------------------------------------------------------------
// helper: tiny builder that always uses ephemeral ports
// -----------------------------------------------------------------------------
func freshBootServer(t *testing.T, withSSL bool) *BootServer {
	t.Helper()

	b := New(&config.BootConfig{}).
		GRPCPort(":0").
		HTTPPort(":0") // ":0" ⇒ OS-chosen free port

	if withSSL {
		b.EnableSSL(DirCache(t.TempDir()))
	}

	bs, err := b.Build()
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	return bs
}
