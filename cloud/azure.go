package cloud

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
)

type Azure struct{}

func (c *Azure) LoadSecretsIntoEnv() {
	logger.Info("Loading Azure Keyvault secrets into environment variables.")
	client := getKeyvaultClient()
	if client == nil {
		logger.Error("Failed to load Azure Keyvault secrets into environment variables.")
		return
	}

	//List secrets
	var secretList []string
	pager := client.NewListSecretsPager(nil)

	for pager.More() {
		page, err := pager.NextPage(context.TODO())
		if err != nil {
			logger.Error("failed to get next page: %v", zap.Error(err))
			return
		}

		for _, secret := range page.Value {
			resp, err := client.GetSecret(context.TODO(), secret.ID.Name(), secret.ID.Version(), nil)
			if err != nil {
				logger.Error("failed to get secret: %v", zap.Error(err))
				continue
			}

			os.Setenv(secret.ID.Name(), *resp.Value)
			secretList = append(secretList, secret.ID.Name())
		}
	}

	logger.Info("Successfully loaded Azure Keyvault secrets into environment variables.", zap.Any("secrets", secretList))
}

// Uploads a stream to Azure storage.
// containerName - Azure Container Name.
// path - Azure path for the object like profile-photos/photo.jpg
func (c *Azure) UploadStream(containerName, path string, imageData bytes.Buffer) (chan string, chan error) {
	resultChan := make(chan string)
	errorChan := make(chan error)

	go func() {
		accountName, accountKey := os.Getenv("AZURE-STORAGE-ACCOUNT"), os.Getenv("AZURE-STORAGE-ACCESS-KEY")
		if len(accountName) == 0 || len(accountKey) == 0 {
			logger.Error("Either the AZURE_STORAGE_ACCOUNT or AZURE_STORAGE_ACCESS_KEY environment variable is not set")
			err := errors.New("missing azure account or access key")
			errorChan <- err
			return
		}

		// Create a default request pipeline using your storage account name and account key.
		credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
		if err != nil {
			logger.Error("Invalid credentials with error: " + err.Error())
			errorChan <- err
			return
		}
		p := azblob.NewPipeline(credential, azblob.PipelineOptions{})

		URL, _ := url.Parse(
			fmt.Sprintf("https://%s.blob.core.windows.net/%s", accountName, containerName))

		containerURL := azblob.NewContainerURL(*URL, p)
		blobURL := containerURL.NewBlockBlobURL(path)
		_, err = azblob.UploadBufferToBlockBlob(context.Background(), imageData.Bytes(), blobURL, azblob.UploadToBlockBlobOptions{
			BlockSize:   4 * 1024 * 1024,
			Parallelism: 16})

		if err != nil {
			logger.Error("Failed uploading image.")
			errorChan <- err
			return
		}

		uploadPath := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", accountName, containerName, path)
		resultChan <- uploadPath
	}()

	return resultChan, errorChan
}

func (c *Azure) GetPresignedUrl(bucket, key string) (string, string) {
	//TODO: Get presigned upload url and download url
	return "", ""
}

func getKeyvaultClient() *azsecrets.Client {
	keyVaultName := os.Getenv("AZURE-KEYVAULT-NAME")
	keyVaultUrl := fmt.Sprintf("https://%s.vault.azure.net/", keyVaultName)

	//Create a credential using the NewDefaultAzureCredential type.
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		logger.Error("failed to obtain a credential: %v", zap.Error(err))
		return nil
	}

	//Establish a connection to the Key Vault client
	client, err := azsecrets.NewClient(keyVaultUrl, cred, nil)
	if err != nil {
		logger.Error("failed to connect to client: %v", zap.Error(err))
		return nil
	}

	return client
}
