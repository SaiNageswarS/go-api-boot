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
	KvClient keyVaultClient

	blobOnce   sync.Once
	blobErr    error
	BlobClient blobClient
}

func ProvideAzure(c *config.BootConfig) Cloud {
	return &Azure{
		ccfgg:      c,
		KvClient:   nil,
		BlobClient: nil,
	}
}

func (a *Azure) LoadSecretsIntoEnv(ctx context.Context) error {
	logger.Info("Loading Azure Keyvault secrets into environment variables.")

	if err := a.EnsureKV(ctx); err != nil {
		logger.Error("Failed to ensure Keyvault client", zap.Error(err))
		return err
	}

	pager := a.KvClient.NewListSecretPropertiesPager(nil)
	var secretList []string

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			logger.Error("Failed to get next page of secrets", zap.Error(err))
			return err
		}
		for _, secret := range page.Value {
			resp, err := a.KvClient.GetSecret(ctx, secret.ID.Name(), secret.ID.Version(), nil)
			if err != nil {
				logger.Error("Failed to get secret", zap.Error(err))
				continue
			}
			_ = os.Setenv(secret.ID.Name(), *resp.Value)
			secretList = append(secretList, secret.ID.Name())
		}
	}

	logger.Info("Successfully loaded Azure Keyvault secrets into environment variables.", zap.Any("secrets", secretList))
	return nil
}

// Uploads a stream to Azure storage.
// containerName - Azure Container Name.
// blobName - Azure path for the object like profile-photos/photo.jpg
func (a *Azure) UploadBuffer(ctx context.Context, containerName, blobName string, fileData []byte) (string, error) {
	if err := a.EnsureBlob(ctx); err != nil {
		logger.Error("failed to ensure blob client", zap.Error(err))
		return "", err
	}

	_, err := a.BlobClient.UploadBuffer(ctx, containerName, blobName, fileData, nil)
	if err != nil {
		logger.Error("failed to upload blob", zap.Error(err))
		return "", err
	}

	url := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", a.ccfgg.AzureStorageAccount, containerName, blobName)
	return url, nil
}

func (a *Azure) DownloadFile(ctx context.Context, containerName, blobName string) (string, error) {
	if err := a.EnsureBlob(ctx); err != nil {
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

	_, err = a.BlobClient.DownloadFile(ctx, containerName, blobName, tmpFile, nil)
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

func (a *Azure) EnsureKV(ctx context.Context) error {
	if a.KvClient != nil {
		return nil
	}

	a.kvOnce.Do(func() {
		a.KvClient, a.kvErr = getKeyvaultClient(a.ccfgg)
	})
	return a.kvErr
}

func (a *Azure) EnsureBlob(ctx context.Context) error {
	if a.BlobClient != nil {
		return nil
	}

	a.blobOnce.Do(func() {
		if a.ccfgg.AzureStorageAccount == "" {
			a.blobErr = errors.New("AzureStorageAccount config not set")
			return
		}

		a.BlobClient, a.blobErr = getServiceClientTokenCredential(a.ccfgg.AzureStorageAccount)
	})
	return a.blobErr
}

func getKeyvaultClient(ccfgg *config.BootConfig) (keyVaultClient, error) {
	// loading from env since config will be initialized after getting secrets from keyvault
	keyVaultName := ccfgg.AzureKeyVaultName
	if keyVaultName == "" {
		return nil, errors.New("azure_key_vault_name config not set")
	}

	keyVaultUrl := fmt.Sprintf("https://%s.vault.azure.net/", keyVaultName)

	cred, err := newDefaultCred()
	if err != nil {
		return nil, err
	}

	client, err := newKVClient(keyVaultUrl, cred)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func getServiceClientTokenCredential(accountName string) (blobClient, error) {
	cred, err := newDefaultCred()
	if err != nil {
		return nil, err
	}

	accountURL := fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
	client, err := newBlobClient(accountURL, cred)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// factory variables – default to real SDK functions
var (
	newDefaultCred = func() (*azidentity.DefaultAzureCredential, error) {
		return azidentity.NewDefaultAzureCredential(nil)
	}
	newKVClient = func(url string, cred *azidentity.DefaultAzureCredential) (keyVaultClient, error) {
		return azsecrets.NewClient(url, cred, nil)
	}
	newBlobClient = func(url string, cred *azidentity.DefaultAzureCredential) (blobClient, error) {
		return azblob.NewClient(url, cred, nil)
	}
)

type keyVaultClient interface {
	NewListSecretPropertiesPager(*azsecrets.ListSecretPropertiesOptions) *runtime.Pager[azsecrets.ListSecretPropertiesResponse]
	GetSecret(context.Context, string, string, *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
}

type blobClient interface {
	UploadBuffer(context.Context, string, string, []byte, *azblob.UploadBufferOptions) (azblob.UploadBufferResponse, error)
	DownloadFile(ctx context.Context, containerName string, blobName string, file *os.File, o *azblob.DownloadFileOptions) (int64, error)
}
