package server

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"time"

	"github.com/SaiNageswarS/go-api-boot/bootUtils"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
	"golang.org/x/crypto/acme/autocert"
)

type SSLManager struct {
	certManager autocert.Manager
	domain      string
}

func NewSSLManager(cache autocert.Cache) *SSLManager {
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		logger.Fatal("DOMAIN environment variable is not set.")
	}

	return &SSLManager{
		certManager: autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(domain), // Your domain name here
			Cache:      cache,
		},

		domain: domain,
	}
}

func (s *SSLManager) DownloadCertificatesWithRetry() {
	getCertificate := func() error {
		cert, err := s.certManager.GetCertificate(&tls.ClientHelloInfo{ServerName: s.domain})
		if err != nil {
			return err
		}
		if cert == nil {
			return autocert.ErrCacheMiss
		}
		return nil
	}

	// Retry getting the certificate with exponential backoff
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	err := bootUtils.RetryWithExponentialBackoff(ctx, 10, 2*time.Second, getCertificate)
	if err != nil {
		logger.Error("Failed to obtain certificate: %v", zap.Error(err))
	}
}

func (s *SSLManager) RunAcmeChallengeListener() {
	go func() {
		// Must run on port 80.
		err := http.ListenAndServe(":http", s.certManager.HTTPHandler(nil))
		if err != nil {
			logger.Error("Failed starting acme challenge listener", zap.Error(err))
		}
	}()
}
