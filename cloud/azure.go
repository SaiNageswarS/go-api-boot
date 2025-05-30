package cloud

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/SaiNageswarS/go-api-boot/config"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
)

type Azure struct {
	ccfgg *config.BootConfig

	kvOnce   sync.Once
	kvErr    error
	kvClient keyVaultClient

	blobOnce   sync.Once
	blobErr    error
	blobClient blobClient
}

func ProvideAzure(c *config.BootConfig) Cloud {
	return &Azure{
		ccfgg:      c,
		kvClient:   nil,
		blobClient: nil,
	}
}

func (a *Azure) LoadSecretsIntoEnv(ctx context.Context) {
	logger.Info("Loading Azure Keyvault secrets into environment variables.")

	if err := a.ensureKV(ctx); err != nil {
		logger.Error("Failed to ensure Keyvault client", zap.Error(err))
		return
	}

	pager := a.kvClient.NewListSecretPropertiesPager(nil)
	var secretList []string

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			logger.Error("Failed to get next page of secrets", zap.Error(err))
			return
		}
		for _, secret := range page.Value {
			resp, err := a.kvClient.GetSecret(ctx, secret.ID.Name(), secret.ID.Version(), nil)
			if err != nil {
				logger.Error("Failed to get secret", zap.Error(err))
				continue
			}
			_ = os.Setenv(secret.ID.Name(), *resp.Value)
			secretList = append(secretList, secret.ID.Name())
		}
	}

	logger.Info("Successfully loaded Azure Keyvault secrets into environment variables.", zap.Any("secrets", secretList))
}

// Uploads a stream to Azure storage.
// containerName - Azure Container Name.
// blobName - Azure path for the object like profile-photos/photo.jpg
func (a *Azure) UploadStream(ctx context.Context, containerName, blobName string, fileData []byte) (string, error) {
	if err := a.ensureBlob(ctx); err != nil {
		logger.Error("failed to ensure blob client", zap.Error(err))
		return "", err
	}

	_, err := a.blobClient.UploadBuffer(ctx, containerName, blobName, fileData, nil)
	if err != nil {
		logger.Error("failed to upload blob", zap.Error(err))
		return "", err
	}

	url := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", a.ccfgg.AzureStorageAccount, containerName, blobName)
	return url, nil
}

func (a *Azure) DownloadFile(ctx context.Context, containerName, blobName string) (string, error) {
	if err := a.ensureBlob(ctx); err != nil {
		logger.Error("failed to ensure blob client", zap.Error(err))
		return "", err
	}

	// Get file name from blob path (e.g., "folder/image.png" → "image.png")
	fileName := path.Base(blobName)

	// Create temp file in the system temp dir
	tmpDir := os.TempDir()
	tmpFilePath := path.Join(tmpDir, fileName)
	tmpFile, err := os.Create(tmpFilePath)
	if err != nil {
		logger.Error("failed to create temp file", zap.Error(err))
		return "", err
	}
	defer tmpFile.Close()

	_, err = a.blobClient.DownloadFile(ctx, containerName, blobName, tmpFile, nil)
	if err != nil {
		logger.Error("failed to download blob to file", zap.Error(err))
		return "", err
	}

	logger.Info("File downloaded successfully", zap.String("filePath", tmpFilePath))
	return tmpFilePath, nil
}

func (c *Azure) GetPresignedUrl(ctx context.Context, bucketName, path, contentType string, expiry time.Duration) (string, string) {
	//TODO: Get presigned upload url and download url
	return "", ""
}

// azure clients

func (a *Azure) ensureKV(ctx context.Context) error {
	if a.kvClient != nil {
		return nil
	}

	a.kvOnce.Do(func() {
		a.kvClient, a.kvErr = getKeyvaultClient(ctx)
	})
	return a.kvErr
}

func (a *Azure) ensureBlob(ctx context.Context) error {
	if a.blobClient != nil {
		return nil
	}

	a.blobOnce.Do(func() {
		if a.ccfgg.AzureStorageAccount == "" {
			a.blobErr = errors.New("AzureStorageAccount config not set")
			return
		}

		a.blobClient, a.blobErr = getServiceClientTokenCredential(ctx, a.ccfgg.AzureStorageAccount)
	})
	return a.blobErr
}

func getKeyvaultClient(ctx context.Context) (keyVaultClient, error) {
	// loading from env since config will be initialized after getting secrets from keyvault
	keyVaultName := os.Getenv("AZURE-KEYVAULT-NAME")
	if keyVaultName == "" {
		return nil, errors.New("AZURE-KEYVAULT-NAME environment variable not set")
	}

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

func getServiceClientTokenCredential(ctx context.Context, accountName string) (blobClient, error) {
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

type keyVaultClient interface {
	NewListSecretPropertiesPager(*azsecrets.ListSecretPropertiesOptions) *runtime.Pager[azsecrets.ListSecretPropertiesResponse]
	GetSecret(context.Context, string, string, *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
}

type blobClient interface {
	UploadBuffer(context.Context, string, string, []byte, *azblob.UploadBufferOptions) (azblob.UploadBufferResponse, error)
	DownloadFile(ctx context.Context, containerName string, blobName string, file *os.File, o *azblob.DownloadFileOptions) (int64, error)
}
