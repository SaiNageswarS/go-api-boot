package server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/SaiNageswarS/go-api-boot/cloud"
	"golang.org/x/crypto/acme/autocert"
)

type SslCloudCache struct {
	cloud cloud.Cloud
}

func NewSslCloudCache(cloud cloud.Cloud) *SslCloudCache {
	return &SslCloudCache{cloud: cloud}
}

func (cc *SslCloudCache) Get(ctx context.Context, name string) ([]byte, error) {
	bucket := os.Getenv("SSL_BUCKET")
	if bucket == "" {
		return nil, fmt.Errorf("SSL_BUCKET environment variable is not set")
	}

	dataChan, errChan := cc.cloud.DownloadFile(bucket, name)
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
	bucket := os.Getenv("SSL_BUCKET")
	if bucket == "" {
		return fmt.Errorf("SSL_BUCKET environment variable is not set")
	}

	resultChan, errChan := cc.cloud.UploadStream(bucket, name, data)
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
