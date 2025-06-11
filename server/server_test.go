package server

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexus-rpc/sdk-go/nexus"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"
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
	_, err := New().
		GRPCPort(":0").
		Build() // missing HTTPPort
	if err == nil {
		t.Fatalf("Build() succeeded with missing HTTPPort; want error")
	}

	_, err = New().
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

	_, err := New().
		GRPCPort(":0").
		HTTPPort(":0").
		EnableSSL(DirCache(t.TempDir())).
		Build()
	if err != nil {
		t.Fatalf("Build() with SSL failed: %v", err)
	}
}

// fakeWorker implements the full worker.Worker interface but only Run is used
type fakeWorker struct {
	runs      int32 // atomically incremented
	failUntil int32 // number of failing attempts before success
}

func (f *fakeWorker) Run(_ <-chan interface{}) error {
	n := atomic.AddInt32(&f.runs, 1)
	if n <= f.failUntil {
		return errors.New("simulated start failure")
	}
	return nil
}

// --- everything below this line is just no-op plumbing to satisfy the interface
func (f *fakeWorker) Start() error { return f.Run(nil) }
func (f *fakeWorker) Stop()        {}

func (f *fakeWorker) RegisterActivity(a interface{})                                              {}
func (f *fakeWorker) RegisterNexusService(_ *nexus.Service)                                       {}
func (f *fakeWorker) RegisterWorkflow(wf interface{})                                             {}
func (f *fakeWorker) RegisterWorkflowWithOptions(w interface{}, options workflow.RegisterOptions) {}

func (f *fakeWorker) RegisterActivityWithOptions(a interface{}, options activity.RegisterOptions) {}

func TestBootServer_Serve_WithTemporalWorker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("CI flakiness on Windows networking, skip")
	}

	// Build a normal BootServer (ports :0) …
	bs := freshBootServer(t, false)

	// …and inject a stub worker that *succeeds immediately* so we don’t wait 10 s
	fw := &fakeWorker{}
	bs.temporalWorker = fw

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = bs.Serve(ctx) // ignore returned error – cancelled below
	}()

	// Give Serve() & the worker a moment to start.
	time.Sleep(100 * time.Millisecond)

	if got := atomic.LoadInt32(&fw.runs); got != 1 {
		t.Fatalf("expected worker.Run to be called once, got %d", got)
	}

	// Cancel everything; Serve should exit promptly.
	cancel()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Serve(ctx) did not return after context cancellation")
	}
}

// -----------------------------------------------------------------------------
// helper: tiny builder that always uses ephemeral ports
// -----------------------------------------------------------------------------
func freshBootServer(t *testing.T, withSSL bool) *BootServer {
	t.Helper()

	b := New().
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
