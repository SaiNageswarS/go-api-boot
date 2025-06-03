package server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/SaiNageswarS/go-api-boot/cloud"
	"github.com/SaiNageswarS/go-api-boot/config"
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
	certFile, err := cc.cloud.DownloadFile(ctx, bkt, name)
	if err != nil {
		return nil, fmt.Errorf("failed to download file %s from bucket %s: %w", name, bkt, err)
	}

	return os.ReadFile(certFile)
}

func (cc *SslCloudCache) Put(ctx context.Context, name string, data []byte) error {
	bkt := cc.cfg.SslBucket
	if bkt == "" {
		return fmt.Errorf("SslBucket not set")
	}
	_, err := cc.cloud.UploadBuffer(ctx, bkt, name, data)
	if err != nil {
		return fmt.Errorf("failed to upload file %s to bucket %s: %w", name, bkt, err)
	}
	return nil
}

func (cc *SslCloudCache) Delete(ctx context.Context, name string) error {
	// not implemented for simplicity
	return nil
}
