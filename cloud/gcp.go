package cloud

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"cloud.google.com/go/storage"
	"github.com/SaiNageswarS/go-api-boot/config"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/googleapis/gax-go/v2"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

type GCP struct {
	ccfgg *config.BootConfig

	secretsOnce sync.Once
	secretsErr  error
	Secrets     secretManagerClient

	storageOnce sync.Once
	storageErr  error
	Storage     storageClient
}

// ProvideGCP returns a GCP cloud client.
func ProvideGCP(ccfgg *config.BootConfig) Cloud {
	return &GCP{ccfgg: ccfgg}
}

// listSecrets lists all secrets in the given project.
func (c *GCP) LoadSecretsIntoEnv(ctx context.Context) error {
	// Create the client.
	if err := c.EnsureSecrets(ctx); err != nil {
		logger.Error("Failed to ensure Secret Manager client", zap.Error(err))
		return err
	}

	// Build the request.
	projectID := c.ccfgg.GcpProjectId
	if projectID == "" {
		logger.Error("gcp_project_id config is not set")
		return fmt.Errorf("gcp_project_id config is not set")
	}
	req := &secretmanagerpb.ListSecretsRequest{
		Parent: fmt.Sprintf("projects/%s", projectID),
	}

	// Call the API.
	it := c.Secrets.ListSecrets(ctx, req)
	var secretList []string
	for {
		secret, err := it.Next()
		if err == iterator.Done {
			break
		}

		if err != nil {
			logger.Error("failed to list secrets: ", zap.Error(err))
			return err
		}

		req := &secretmanagerpb.AccessSecretVersionRequest{
			Name: fmt.Sprintf("%s/versions/latest", secret.Name),
		}
		result, err := c.Secrets.AccessSecretVersion(ctx, req)
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
	return nil
}

func (c *GCP) UploadBuffer(ctx context.Context, bucketName, path string, fileData []byte) (string, error) {
	// Set up the Google Cloud Storage client
	if err := c.EnsureStorage(ctx); err != nil {
		logger.Error("Failed to ensure Storage client", zap.Error(err))
		return "", err
	}

	bucket := c.Storage.Bucket(bucketName)
	obj := bucket.Object(path)
	wc := obj.NewWriter(ctx)

	// Copy the contents of the buffer to the object in Cloud Storage.
	fileReader := bytes.NewReader(fileData)
	if _, err := io.Copy(wc, fileReader); err != nil {
		wc.Close()
		return "", err
	}

	// Close the Writer, finalizing the upload.
	if err := wc.Close(); err != nil {
		return "", err
	}

	// Get the public URL for the object.
	objectURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, path)
	return objectURL, nil
}

// DownloadFile downloads a file from GCP bucket and returns the path to the temp file.
func (c *GCP) DownloadFile(ctx context.Context, bucketName, blobPath string) (string, error) {
	if err := c.EnsureStorage(ctx); err != nil {
		logger.Error("Failed to ensure Storage client", zap.Error(err))
		return "", err
	}

	bucket := c.Storage.Bucket(bucketName)
	obj := bucket.Object(blobPath)

	r, err := obj.NewReader(ctx)
	if err != nil {
		return "", fmt.Errorf("obj.NewReader: %v", err)
	}
	defer r.Close()

	// Get file name from blob path (e.g., "folder/image.png" → "image.png")
	fileName := path.Base(blobPath)

	// Create temp file in the system temp dir
	tmpDir := os.TempDir()
	tmpFilePath := path.Join(tmpDir, fileName)
	tempFile, err := os.Create(tmpFilePath)
	if err != nil {
		return "", fmt.Errorf("os.Create: %v", err)
	}
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, r); err != nil {
		return "", fmt.Errorf("io.Copy: %v", err)
	}

	return tmpFilePath, nil
}

func (c *GCP) GetPresignedUrl(ctx context.Context, bucketName, path, contentType string, expiry time.Duration) (string, string) {
	// bucketName := "bucket-name"
	// path := "object-name"

	if err := c.EnsureStorage(ctx); err != nil {
		logger.Error("Failed to ensure Storage client", zap.Error(err))
		return "", ""
	}

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

	uploadUrl, err := c.Storage.Bucket(bucketName).SignedURL(path, opts)
	if err != nil {
		logger.Error("Failed to generate signed URL: ", zap.Error(err))
		return "", ""
	}
	downloadUrl := fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, path)
	return uploadUrl, downloadUrl
}

func (c *GCP) EnsureSecrets(ctx context.Context) error {
	c.secretsOnce.Do(func() {
		c.Secrets, c.secretsErr = newSecrets(ctx)
	})
	return c.secretsErr
}

func (c *GCP) EnsureStorage(ctx context.Context) error {
	c.storageOnce.Do(func() {
		c.Storage, c.storageErr = newStorage(ctx)
	})
	return c.storageErr
}

var (
	newStorage = func(ctx context.Context) (storageClient, error) {
		return storage.NewClient(ctx)
	}
	newSecrets = func(ctx context.Context) (secretManagerClient, error) {
		return secretmanager.NewClient(ctx)
	}
)

type secretManagerClient interface {
	ListSecrets(ctx context.Context, req *secretmanagerpb.ListSecretsRequest, opts ...gax.CallOption) *secretmanager.SecretIterator
	AccessSecretVersion(ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error)
}

type storageClient interface {
	Bucket(name string) *storage.BucketHandle
}
