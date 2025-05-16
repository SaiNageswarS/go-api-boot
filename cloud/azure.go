package cloud

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	azsecrets "github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	azblob "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
)

// Azure is the public implementation of the Cloud interface
// It uses injected clients for testability
type Azure struct {
	KeyVaultClient KeyVaultClient
	StorageClient  StorageUploader
}

// ProvideAzure returns an instance of Azure with nil clients initially (created lazily)
func ProvideAzure() Cloud {
	return &Azure{}
}

func (a *Azure) LoadSecretsIntoEnv() {
	logger.Info("Loading Azure Keyvault secrets into environment variables.")

	if a.KeyVaultClient == nil {
		keyVaultName := os.Getenv("AZURE-KEYVAULT-NAME")
		if keyVaultName == "" {
			logger.Error("AZURE-KEYVAULT-NAME not set")
			return
		}
		url := fmt.Sprintf("https://%s.vault.azure.net/", keyVaultName)

		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			logger.Error("Failed to get Azure credential", zap.Error(err))
			return
		}

		client, err := azsecrets.NewClient(url, cred, nil)
		if err != nil {
			logger.Error("Failed to create KeyVault client", zap.Error(err))
			return
		}
		a.KeyVaultClient = &realKeyVaultClient{client}
	}

	client := a.KeyVaultClient
	var secretList []string
	pager := client.ListSecrets(nil)

	for pager.More() {
		page, err := pager.NextPage(context.TODO())
		if err != nil {
			logger.Error("failed to get next page", zap.Error(err))
			return
		}

		for _, secret := range page.Value {
			name := *secret.ID.Name
			resp, err := client.GetSecret(context.TODO(), name, "", nil)
			if err != nil {
				logger.Error("failed to get secret", zap.Error(err))
				continue
			}
			os.Setenv(name, *resp.Value)
			secretList = append(secretList, name)
		}
	}

	logger.Info("Loaded Azure secrets into env", zap.Any("secrets", secretList))
}

func (a *Azure) UploadStream(container, path string, buf bytes.Buffer) (chan string, chan error) {
	resultChan := make(chan string)
	errorChan := make(chan error)

	go func() {
		if a.StorageClient == nil {
			accountName := os.Getenv("AZURE-STORAGE-ACCOUNT")
			accountKey := os.Getenv("AZURE-STORAGE-ACCESS-KEY")

			if accountName == "" || accountKey == "" {
				errorChan <- errors.New("missing azure account or key")
				return
			}

			cred, err := azblob.NewSharedKeyCredential(accountName, accountKey)
			if err != nil {
				errorChan <- err
				return
			}

			uploader := &realStorageUploader{accountName: accountName, credential: cred}
			a.StorageClient = uploader
		}

		url, err := a.StorageClient.Upload(container, path, buf.Bytes())
		if err != nil {
			errorChan <- err
			return
		}
		resultChan <- url
	}()

	return resultChan, errorChan
}

func (a *Azure) GetPresignedUrl(bucketName, path, contentType string, expiry time.Duration) (string, string) {
	// Not implemented
	return "", ""
}

// Interfaces for injection

type KeyVaultClient interface {
	ListSecrets(*azsecrets.ListSecretsOptions) *runtime.Pager[azsecrets.ListSecretsResponse]
	GetSecret(ctx context.Context, name, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
}

type StorageUploader interface {
	Upload(container, path string, data []byte) (string, error)
}

// Real production implementations

type realKeyVaultClient struct {
	client *azsecrets.Client
}

func (r *realKeyVaultClient) ListSecrets(opts *azsecrets.ListSecretsOptions) *runtime.Pager[azsecrets.ListSecretsResponse] {
	return r.client.NewListSecretsPager(opts)
}

func (r *realKeyVaultClient) GetSecret(ctx context.Context, name, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
	return r.client.GetSecret(ctx, name, version, options)
}

type realStorageUploader struct {
	accountName string
	credential  *azblob.SharedKeyCredential
}

func (r *realStorageUploader) Upload(container, path string, data []byte) (string, error) {
	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", r.accountName)
	containerClient, err := azblob.NewContainerClient(fmt.Sprintf("%s%s", serviceURL, container), r.credential, nil)
	if err != nil {
		return "", err
	}

	blobClient := containerClient.NewBlockBlobClient(path)
	_, err = blobClient.UploadBuffer(context.Background(), data, azblob.UploadOption{})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s%s/%s", serviceURL, container, path), nil
}
