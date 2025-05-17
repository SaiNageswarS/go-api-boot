package cloud

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

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
		return
	}
	defer client.Close()

	// Build the request.
	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		logger.Error("GCP_PROJECT_ID environment variable is not set.")
		return
	}
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
			return
		}

		req := &secretmanagerpb.AccessSecretVersionRequest{
			Name: fmt.Sprintf("%s/versions/latest", secret.Name),
		}
		result, err := client.AccessSecretVersion(ctx, req)
		if err != nil {
			logger.Error("Failed to access secret version for:", zap.Any("secret version", secret.Name), zap.Error(err))
			continue
		}

		// Extract the secret name and value.
		secretValue := string(result.Payload.Data)
		secretName := secret.Name[strings.LastIndex(secret.Name, "/")+1:]

		os.Setenv(secretName, secretValue)
		secretList = append(secretList, secretName)
	}
	logger.Info("Successfully loaded GCP Keyvault secrets into environment variables.", zap.Any("secrets", secretList))
}

func (c *GCP) UploadStream(bucketName, path string, fileData []byte) (chan string, chan error) {
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
		fileReader := bytes.NewReader(fileData)
		if _, err := io.Copy(wc, fileReader); err != nil {
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

// DownloadFile downloads a file from GCP bucket and returns the path to the temp file.
func (c *GCP) DownloadFile(bucketName, blobPath string) (chan string, chan error) {
	resultChan := make(chan string)
	errorChan := make(chan error)

	go func() {
		client, err := storage.NewClient(context.Background())
		if err != nil {
			errorChan <- fmt.Errorf("storage.NewClient: %v", err)
			return
		}
		defer client.Close()

		bucket := client.Bucket(bucketName)
		obj := bucket.Object(blobPath)

		r, err := obj.NewReader(context.Background())
		if err != nil {
			errorChan <- fmt.Errorf("Object.NewReader: %v", err)
			return
		}
		defer r.Close()

		// Get file name from blob path (e.g., "folder/image.png" → "image.png")
		fileName := path.Base(blobPath)

		// Create temp file in the system temp dir
		tmpDir := os.TempDir()
		tmpFilePath := path.Join(tmpDir, fileName)
		tempFile, err := os.Create(tmpFilePath)
		if err != nil {
			errorChan <- fmt.Errorf("os.CreateTemp: %v", err)
			return
		}
		defer tempFile.Close()

		if _, err := io.Copy(tempFile, r); err != nil {
			errorChan <- fmt.Errorf("io.Copy: %v", err)
			return
		}

		resultChan <- tempFile.Name()
	}()

	return resultChan, errorChan
}

func (c *GCP) GetPresignedUrl(bucketName, path, contentType string, expiry time.Duration) (string, string) {
	// bucketName := "bucket-name"
	// path := "object-name"

	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		logger.Error("Failed to create client: ", zap.Error(err))
		return "", ""
	}
	defer client.Close()

	// Signing a URL requires credentials authorized to sign a URL. You can pass
	// these in through SignedURLOptions with one of the following options:
	//    a. a Google service account private key, obtainable from the Google Developers Console
	//    b. a Google Access ID with iam.serviceAccounts.signBlob permissions
	//    c. a SignBytes function implementing custom signing.
	// As nothing is passed default option is used which is same as the method used to initialise the client.
	opts := &storage.SignedURLOptions{
		Scheme: storage.SigningSchemeV4,
		Method: "PUT",
		Headers: []string{
			fmt.Sprintf("Content-Type:%s", contentType),
		},
		Expires: time.Now().Add(expiry),
	}

	uploadUrl, err := client.Bucket(bucketName).SignedURL(path, opts)
	if err != nil {
		logger.Error("Failed to generate signed URL: ", zap.Error(err))
		return "", ""
	}
	downloadUrl := fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, path)
	return uploadUrl, downloadUrl
}
