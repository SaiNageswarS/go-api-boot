package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/SaiNageswarS/go-api-boot/bootUtils"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
	"golang.org/x/crypto/acme/autocert"
)

type SSLManager struct {
	certManager *autocert.Manager
	httpSrv     *http.Server // ACME HTTP-01 listener (port 80)
	domain      string
}

// NewSSLManager uses any autocert.Cache (dir, cloud, memory…)
func NewSSLManager(domain string, cache autocert.Cache) *SSLManager {
	mgr := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domain),
		Cache:      cache,
	}
	return &SSLManager{
		certManager: mgr,
		domain:      domain,
	}
}

/* ---------- SSLProvider implementation ---------- */

// 1) Wire GetCertificate into the server we’ll expose publicly.
func (s *SSLManager) Configure(srv *http.Server) error {
	if srv.TLSConfig == nil {
		srv.TLSConfig = &tls.Config{}
	}
	srv.TLSConfig.GetCertificate = s.certManager.GetCertificate
	return nil
}

//  2. Run ACME helper: listener on :80 + cert pre-fetch w/ backoff.
//     Returns when ctx is cancelled.
func (s *SSLManager) Run(ctx context.Context) error {
	// a) Challenge listener (port 80)
	ln, err := net.Listen("tcp", ":http") // ":http" == ":80"
	if err != nil {
		return fmt.Errorf("acme listener: %w", err)
	}
	s.httpSrv = &http.Server{
		Handler: s.certManager.HTTPHandler(nil),
	}
	go func() {
		_ = s.httpSrv.Serve(ln) // shuts down via ctx
	}()

	// b) Prefetch/renew certificate with retry & back-off
	err = bootUtils.RetryWithExponentialBackoff(ctx, 10, 2*time.Second, func() error {
		_, e := s.certManager.GetCertificate(
			&tls.ClientHelloInfo{ServerName: s.domain},
		)
		return e
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("acme: certificate fetch", zap.Error(err))
	}

	// Wait for cancellation
	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.httpSrv.Shutdown(shutCtx)
	return nil
}
