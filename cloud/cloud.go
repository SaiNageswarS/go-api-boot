package cloud

import "bytes"

type Cloud interface {
	LoadSecretsIntoEnv()
	UploadStream(bucketName, path string, imageData bytes.Buffer) (chan string, chan error)
	GetPresignedUrl(bucketName, path string) (string, string)
}
