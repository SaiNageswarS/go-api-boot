package cloud

import (
	"time"

	"github.com/SaiNageswarS/go-api-boot/config"
)

type Cloud interface {
	LoadSecretsIntoEnv()
	UploadStream(config *config.BootConfig, bucketName, path string, fileData []byte) (chan string, chan error)
	GetPresignedUrl(config *config.BootConfig, bucketName, path, contentType string, expiry time.Duration) (string, string)
	DownloadFile(config *config.BootConfig, bucketName, path string) (chan string, chan error)
}
