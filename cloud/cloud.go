package cloud

import (
	"time"
)

type Cloud interface {
	LoadSecretsIntoEnv()
	UploadStream(bucketName, path string, fileData []byte) (chan string, chan error)
	GetPresignedUrl(bucketName, path, contentType string, expiry time.Duration) (string, string)
	DownloadFile(bucketName, path string) (chan string, chan error)
}
