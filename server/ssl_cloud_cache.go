package server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/SaiNageswarS/go-api-boot/cloud"
	"github.com/SaiNageswarS/go-api-boot/config"
	"golang.org/x/crypto/acme/autocert"
)

type SslCloudCache struct {
	cloud cloud.Cloud
	cfg   *config.BootConfig
	ttl   time.Duration // optional: default 10 s
}

// ctor
func NewSslCloudCache(cfg *config.BootConfig, cloud cloud.Cloud) *SslCloudCache {
	return &SslCloudCache{cloud: cloud, cfg: cfg, ttl: 10 * time.Second}
}

func (cc *SslCloudCache) Get(ctx context.Context, name string) ([]byte, error) {
	bkt := cc.cfg.SslBucket
	if bkt == "" {
		return nil, fmt.Errorf("SslBucket not set")
	}
	dC, eC := cc.cloud.DownloadFile(cc.cfg, bkt, name)
	select {
	case p := <-dC:
		return os.ReadFile(p)
	case err := <-eC:
		return nil, err
	case <-time.After(cc.ttl):
		return nil, autocert.ErrCacheMiss
	}
}

func (cc *SslCloudCache) Put(ctx context.Context, name string, data []byte) error {
	bkt := cc.cfg.SslBucket
	if bkt == "" {
		return fmt.Errorf("SslBucket not set")
	}
	rC, eC := cc.cloud.UploadStream(cc.cfg, bkt, name, data)
	select {
	case <-rC:
		return nil
	case err := <-eC:
		return err
	case <-time.After(cc.ttl):
		return fmt.Errorf("upload timed out")
	}
}

func (cc *SslCloudCache) Delete(ctx context.Context, name string) error {
	// not implemented for simplicity
	return nil
}
