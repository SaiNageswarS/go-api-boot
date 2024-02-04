package gcp

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
)

var GCPStorage storageWrapper = storageWrapper{}

type storageWrapper struct{}

func (s storageWrapper) UploadStream(bucketName, objectPath string, buffer *bytes.Buffer) (chan string, chan error) {
	resultChan := make(chan string)
	errChan := make(chan error)

	go func() {

		// Set up the Google Cloud Storage client
		client, err := storage.NewClient(context.Background())
		if err != nil {
			errChan <- fmt.Errorf("storage.NewClient: %v", err)
			return
		}
		defer client.Close()

		bucket := client.Bucket(bucketName)
		obj := bucket.Object(objectPath)
		wc := obj.NewWriter(context.Background())

		// Copy the contents of the buffer to the object in Cloud Storage.
		if _, err := io.Copy(wc, buffer); err != nil {
			wc.Close()
			errChan <- fmt.Errorf("io.Copy: %v", err)
			return
		}

		// Close the Writer, finalizing the upload.
		if err := wc.Close(); err != nil {
			errChan <- fmt.Errorf("Writer.Close: %v", err)
			return
		}

		// Get the public URL for the object.
		objectURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, objectPath)

		resultChan <- objectURL
	}()

	// The function returns immediately, and the actual upload happens in the goroutine.
	return resultChan, errChan
}
