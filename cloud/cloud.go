package cloud

import (
	"bytes"
	"time"
)

type Cloud interface {
	LoadSecretsIntoEnv()
	UploadStream(bucketName, path string, imageData bytes.Buffer) (chan string, chan error)
	GetPresignedUrl(bucketName, path string, expiry time.Duration) (string, string)
}
