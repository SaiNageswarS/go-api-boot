package server

import (
	"context"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/config"
)

/*─────────────────────────────────────────────────────────────────────────────
  DirCache provider
─────────────────────────────────────────────────────────────────────────────*/

func TestDirCache_ConfigureSetsTLSConfig(t *testing.T) {
	p := DirCache(t.TempDir())

	srv := &http.Server{}
	if err := p.Configure(srv); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	if srv.TLSConfig == nil {
		t.Fatalf("Configure() did not allocate TLSConfig")
	}
	if srv.TLSConfig.GetCertificate == nil {
		t.Fatalf("TLSConfig.GetCertificate not set")
	}
}

func TestDirCache_Run_ReturnsOnCancel(t *testing.T) {
	p := DirCache(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		_ = p.Run(ctx) // Run should block until ctx cancelled
		close(done)
	}()

	// trigger cancellation; Run must return promptly
	cancel()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("Run(ctx) did not return after cancellation")
	}
}

/*─────────────────────────────────────────────────────────────────────────────
  CloudCacheProvider  (uses SSLManager internally)
─────────────────────────────────────────────────────────────────────────────*/

func TestCloudCacheProvider_Configure(t *testing.T) {
	cfg := &config.BootConfig{Domain: "example.com", SslBucket: "dummy"}
	p := CloudCacheProvider(cfg, &mockCloud{})

	srv := &http.Server{}
	if err := p.Configure(srv); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	if srv.TLSConfig == nil || srv.TLSConfig.GetCertificate == nil {
		t.Fatalf("TLSConfig not properly initialised by provider")
	}
}

func TestCloudCacheProvider_Run_ReturnsOnCancel(t *testing.T) {
	// On most CI runners, opening :80 will fail quickly – that is fine.
	if runtime.GOOS == "windows" {
		t.Skip("ACME :http listener unsupported on Windows CI")
	}

	cfg := &config.BootConfig{Domain: "example.com", SslBucket: "dummy"}
	p := CloudCacheProvider(cfg, &mockCloud{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		_ = p.Run(ctx) // ignore error; may fail on port 80
		close(done)
	}()

	time.Sleep(50 * time.Millisecond) // let Run start its goroutine(s)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("Run(ctx) did not return after cancellation")
	}
}
