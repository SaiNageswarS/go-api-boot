package cloud

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"cloud.google.com/go/storage"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

type GCP struct{}

// listSecrets lists all secrets in the given project.
func (c *GCP) LoadSecretsIntoEnv() {

	// Create the client.
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		logger.Error("Failed to create client: ", zap.Error(err))
	}
	defer client.Close()

	// Build the request.
	projectID := os.Getenv("GCP-PROJECT-ID")
	req := &secretmanagerpb.ListSecretsRequest{
		Parent: fmt.Sprintf("projects/%s", projectID),
	}

	// Call the API.
	it := client.ListSecrets(ctx, req)
	var secretList []string
	for {
		secret, err := it.Next()
		if err == iterator.Done {
			break
		}

		if err != nil {
			logger.Error("failed to list secrets: ", zap.Error(err))
		}

		req := &secretmanagerpb.AccessSecretVersionRequest{
			Name: fmt.Sprintf("%s/versions/latest", secret.Name),
		}
		result, err := client.AccessSecretVersion(ctx, req)
		if err != nil {
			logger.Error("Failed to access secret version for:", zap.Any("secret version", secret.Name), zap.Error(err))
		}

		// Extract the secret name and value.
		secretValue := string(result.Payload.Data)
		secretName := secret.Name[strings.LastIndex(secret.Name, "/")+1:]

		os.Setenv(secretName, secretValue)
		fmt.Println("secretName", secretName, secretValue)
		secretList = append(secretList, secretName)
	}
	logger.Info("Successfully loaded GCP Keyvault secrets into environment variables.", zap.Any("secrets", secretList))
}

func (c *GCP) UploadStream(bucketName, path string, imageData bytes.Buffer) (chan string, chan error) {
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
		obj := bucket.Object(path)
		wc := obj.NewWriter(context.Background())

		// Copy the contents of the buffer to the object in Cloud Storage.
		if _, err := io.Copy(wc, &imageData); err != nil {
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
		objectURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, path)

		resultChan <- objectURL
	}()

	// The function returns immediately, and the actual upload happens in the goroutine.
	return resultChan, errChan
}
