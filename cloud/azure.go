package cloud

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/SaiNageswarS/go-api-boot/config"
	"github.com/SaiNageswarS/go-api-boot/dotenv"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
)

type Azure struct {
	// unexported fields only used in test
	overrideVaultClient func() (KeyVaultClient, error)
	overrideBlobClient  func(string) (BlobClient, error)
	overrideSetEnv      func(string, string) error
}

func (a *Azure) LoadSecretsIntoEnv() {
	logger.Info("Loading Azure Keyvault secrets into environment variables.")

	var client KeyVaultClient
	var err error
	if a.overrideVaultClient != nil {
		client, err = a.overrideVaultClient()
	} else {
		client, err = getKeyvaultClient()
	}
	if err != nil {
		logger.Error("Failed to initialize Keyvault client", zap.Error(err))
		return
	}

	setEnv := os.Setenv
	if a.overrideSetEnv != nil {
		setEnv = a.overrideSetEnv
	}

	pager := client.NewListSecretPropertiesPager(nil)
	var secretList []string

	for pager.More() {
		page, err := pager.NextPage(context.TODO())
		if err != nil {
			logger.Error("Failed to get next page of secrets", zap.Error(err))
			return
		}
		for _, secret := range page.Value {
			resp, err := client.GetSecret(context.TODO(), secret.ID.Name(), secret.ID.Version(), nil)
			if err != nil {
				logger.Error("Failed to get secret", zap.Error(err))
				continue
			}
			_ = setEnv(secret.ID.Name(), *resp.Value)
			secretList = append(secretList, secret.ID.Name())
		}
	}

	logger.Info("Successfully loaded Azure Keyvault secrets into environment variables.", zap.Any("secrets", secretList))
}

// Uploads a stream to Azure storage.
// containerName - Azure Container Name.
// blobName - Azure path for the object like profile-photos/photo.jpg
func (a *Azure) UploadStream(config *config.BootConfig, containerName, blobName string, fileData []byte) (chan string, chan error) {
	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	go func() {
		accountName := config.AzureStorageAccount
		if accountName == "" {
			errorChan <- fmt.Errorf("AzureStorageAccount config not set")
			return
		}

		var client BlobClient
		var err error
		if a.overrideBlobClient != nil {
			client, err = a.overrideBlobClient(accountName)
		} else {
			client, err = getServiceClientTokenCredential(accountName)
		}

		if err != nil || client == nil {
			errorChan <- fmt.Errorf("failed to create blob client: %v", err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		_, err = client.UploadBuffer(ctx, containerName, blobName, fileData, nil)
		if err != nil {
			logger.Error("failed to upload blob", zap.Error(err))
			errorChan <- err
			return
		}

		url := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", accountName, containerName, blobName)
		resultChan <- url
	}()

	return resultChan, errorChan
}

func (a *Azure) DownloadFile(config *config.BootConfig, containerName, blobName string) (chan string, chan error) {
	dotenv.LoadEnv()
	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	go func() {
		accountName := config.AzureStorageAccount
		if accountName == "" {
			errorChan <- fmt.Errorf("AzureStorageAccount config not set")
			return
		}

		var client BlobClient
		var err error
		if a.overrideBlobClient != nil {
			client, err = a.overrideBlobClient(accountName)
		} else {
			client, err = getServiceClientTokenCredential(accountName)
		}

		if err != nil || client == nil {
			errorChan <- fmt.Errorf("failed to create blob client: %v", err)
			return
		}

		// Get file name from blob path (e.g., "folder/image.png" → "image.png")
		fileName := path.Base(blobName)

		// Create temp file in the system temp dir
		tmpDir := os.TempDir()
		tmpFilePath := path.Join(tmpDir, fileName)
		tmpFile, err := os.Create(tmpFilePath)
		if err != nil {
			errorChan <- fmt.Errorf("failed to create temp file: %v", err)
			return
		}
		defer tmpFile.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		_, err = client.DownloadFile(ctx, containerName, blobName, tmpFile, nil)
		if err != nil {
			logger.Error("failed to download blob to file", zap.Error(err))
			errorChan <- err
			return
		}

		resultChan <- tmpFilePath
	}()

	return resultChan, errorChan
}

func (c *Azure) GetPresignedUrl(config *config.BootConfig, bucketName, path, contentType string, expiry time.Duration) (string, string) {
	//TODO: Get presigned upload url and download url
	return "", ""
}

// azure clients
func getKeyvaultClient() (KeyVaultClient, error) {
	keyVaultName := os.Getenv("AZURE-KEYVAULT-NAME")
	keyVaultUrl := fmt.Sprintf("https://%s.vault.azure.net/", keyVaultName)

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get default credential: %w", err)
	}

	client, err := azsecrets.NewClient(keyVaultUrl, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyvault client: %w", err)
	}
	return client, nil
}

func getServiceClientTokenCredential(accountName string) (BlobClient, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get credential: %w", err)
	}

	accountURL := fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
	client, err := azblob.NewClient(accountURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create blob client: %w", err)
	}

	return client, nil
}

type KeyVaultClient interface {
	NewListSecretPropertiesPager(*azsecrets.ListSecretPropertiesOptions) *runtime.Pager[azsecrets.ListSecretPropertiesResponse]
	GetSecret(context.Context, string, string, *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
}

type BlobClient interface {
	UploadBuffer(context.Context, string, string, []byte, *azblob.UploadBufferOptions) (azblob.UploadBufferResponse, error)
	DownloadFile(ctx context.Context, containerName string, blobName string, file *os.File, o *azblob.DownloadFileOptions) (int64, error)
}
