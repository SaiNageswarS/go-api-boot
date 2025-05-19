package server

import (
	"context"
	"crypto/tls"
	"net/http"

	"github.com/SaiNageswarS/go-api-boot/cloud"
	"github.com/SaiNageswarS/go-api-boot/config"
	"golang.org/x/crypto/acme/autocert"
)

type SSLProvider interface {
	// Configure mutates srv.TLSConfig so that http.Server can serve TLS.
	Configure(srv *http.Server) error

	// Run launches any background logic the provider needs
	// (ACME challenge listener, certificate refresh, etc.).
	// It must return when ctx is cancelled.
	Run(ctx context.Context) error
}

// Choose one of the following implementations of SSLProvider

// Dir cache provider (1-liner)
func DirCache(dir string) SSLProvider {
	return &dirCache{dir: dir}
}

type dirCache struct{ dir string }

func (d *dirCache) Configure(srv *http.Server) error {
	m := autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Cache:  autocert.DirCache(d.dir),
	}
	if srv.TLSConfig == nil {
		srv.TLSConfig = &tls.Config{}
	}
	srv.TLSConfig.GetCertificate = m.GetCertificate
	return nil
}
func (d *dirCache) Run(ctx context.Context) error { <-ctx.Done(); return nil }

// Cloud provider (wraps SSLManager with SslCloudCache)
func CloudCacheProvider(cfg *config.BootConfig, cloud cloud.Cloud) SSLProvider {
	domain := cfg.Domain // or os.Getenv("DOMAIN")
	return NewSSLManager(domain, NewSslCloudCache(cfg, cloud))
}
