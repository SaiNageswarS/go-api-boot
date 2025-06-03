package cloud

import (
	"context"
	"time"
)

type Cloud interface {
	LoadSecretsIntoEnv(ctx context.Context)
	UploadBuffer(ctx context.Context, bucketName, path string, fileData []byte) (string, error)
	GetPresignedUrl(ctx context.Context, bucketName, path, contentType string, expiry time.Duration) (string, string)
	DownloadFile(ctx context.Context, bucketName, path string) (string, error)
}
