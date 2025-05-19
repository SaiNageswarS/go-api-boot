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
	cloud  cloud.Cloud
	config *config.BootConfig
}

func NewSslCloudCache(config *config.BootConfig, cloud cloud.Cloud) *SslCloudCache {
	return &SslCloudCache{config: config, cloud: cloud}
}

func (cc *SslCloudCache) Get(ctx context.Context, name string) ([]byte, error) {
	bucket := cc.config.SslBucket
	if bucket == "" {
		return nil, fmt.Errorf("SslBucket config is not set")
	}

	dataChan, errChan := cc.cloud.DownloadFile(cc.config, bucket, name)
	select {
	case dataPath := <-dataChan:
		return os.ReadFile(dataPath)
	case err := <-errChan:
		return nil, err
	case <-time.After(10 * time.Second):
		return nil, autocert.ErrCacheMiss
	}
}

func (cc *SslCloudCache) Put(ctx context.Context, name string, data []byte) error {
	bucket := cc.config.SslBucket
	if bucket == "" {
		return fmt.Errorf("SslBucket config is not set")
	}

	resultChan, errChan := cc.cloud.UploadStream(cc.config, bucket, name, data)
	select {
	case <-resultChan:
		return nil
	case err := <-errChan:
		return err
	case <-time.After(10 * time.Second):
		return fmt.Errorf("upload timed out")
	}
}

func (cc *SslCloudCache) Delete(ctx context.Context, key string) error {
	// Not implemented for simplicity.
	return nil
}
