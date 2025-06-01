package cloud

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/SaiNageswarS/go-api-boot/config"
	"github.com/stretchr/testify/assert"
)

func TestAzure_LoadSecretsIntoEnv(t *testing.T) {
	mockSecrets := map[string]string{
		"FOO": "bar",
		"BAZ": "qux",
	}
	os.Clearenv() // Clear existing environment variables

	a := &Azure{
		KvClient: &mockVaultClient{
			secrets: mockSecrets,
		},
	}

	a.LoadSecretsIntoEnv(context.Background())

	assert.Equal(t, "bar", os.Getenv("FOO"))
	assert.Equal(t, "qux", os.Getenv("BAZ"))
}

func TestAzure_UploadStream_Success(t *testing.T) {
	config := &config.BootConfig{
		AzureStorageAccount: "mystorage",
	}

	a := &Azure{
		ccfgg:      config,
		BlobClient: &mockBlobClient{},
	}

	url, err := a.UploadStream(context.Background(), "container", "myblob.txt", []byte("test content"))

	assert.NoError(t, err)
	assert.Contains(t, url, "https://mystorage.blob.core.windows.net/container/myblob.txt")
}

func TestAzure_UploadStream_Failure(t *testing.T) {
	config := &config.BootConfig{
		AzureStorageAccount: "mystorage",
	}

	a := &Azure{
		ccfgg:      config,
		BlobClient: &mockBlobClient{ShouldFail: true},
	}

	url, err := a.UploadStream(context.Background(), "container", "myblob.txt", []byte("test content"))

	assert.Error(t, err)
	assert.EqualError(t, err, "upload failed")
	assert.Empty(t, url)
}

func TestAzure_DownloadFile_Success(t *testing.T) {
	config := &config.BootConfig{
		AzureStorageAccount: "mystorage",
	}

	a := &Azure{
		ccfgg:      config,
		BlobClient: &mockBlobClient{},
	}

	filePath, err := a.DownloadFile(context.Background(), "container", "path/to/blob.txt")

	assert.NoError(t, err)
	assert.Contains(t, filePath, "blob.txt")
	content, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	assert.Equal(t, "mock blob content", string(content))
	_ = os.Remove(filePath) // cleanup
}

func TestAzure_DownloadFile_Failure(t *testing.T) {
	config := &config.BootConfig{
		AzureStorageAccount: "mystorage",
	}

	a := &Azure{
		ccfgg:      config,
		BlobClient: &mockBlobClient{ShouldFail: true},
	}

	filePath, err := a.DownloadFile(context.Background(), "container", "blob.txt")

	assert.Error(t, err)
	assert.EqualError(t, err, "download failed")
	assert.Empty(t, filePath)
}

type mockVaultClient struct {
	secrets map[string]string
}

func (m *mockVaultClient) NewListSecretPropertiesPager(*azsecrets.ListSecretPropertiesOptions) *runtime.Pager[azsecrets.ListSecretPropertiesResponse] {
	keys := make([]string, 0, len(m.secrets))
	for k := range m.secrets {
		keys = append(keys, k)
	}
	index := 0

	return runtime.NewPager(runtime.PagingHandler[azsecrets.ListSecretPropertiesResponse]{
		More: func(resp azsecrets.ListSecretPropertiesResponse) bool {
			return index < len(keys)
		},
		Fetcher: func(ctx context.Context, _ *azsecrets.ListSecretPropertiesResponse) (azsecrets.ListSecretPropertiesResponse, error) {
			if index >= len(keys) {
				return azsecrets.ListSecretPropertiesResponse{}, nil
			}

			secretName := keys[index]
			index++

			// Simulate the full URL (format used in actual Azure response)
			fullID := azsecrets.ID(fmt.Sprintf("https://fake-vault.vault.azure.net/secrets/%s/123456", secretName))

			return azsecrets.ListSecretPropertiesResponse{
				SecretPropertiesListResult: azsecrets.SecretPropertiesListResult{
					Value: []*azsecrets.SecretProperties{
						{
							ID: &fullID,
						},
					},
				},
			}, nil
		},
	})
}

func (m *mockVaultClient) GetSecret(ctx context.Context, name string, version string, secretOptions *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
	if val, ok := m.secrets[name]; ok {
		return azsecrets.GetSecretResponse{Secret: azsecrets.Secret{Value: &val}}, nil
	}

	return azsecrets.GetSecretResponse{}, errors.New("not found")
}

type mockBlobClient struct {
	ShouldFail bool
}

func (m *mockBlobClient) UploadBuffer(ctx context.Context, containerName string, blobName string, data []byte, opts *azblob.UploadBufferOptions) (azblob.UploadBufferResponse, error) {
	if m.ShouldFail {
		return azblob.UploadBufferResponse{}, errors.New("upload failed")
	}
	return azblob.UploadBufferResponse{}, nil
}

func (m *mockBlobClient) DownloadFile(ctx context.Context, containerName string, blobName string, file *os.File, o *azblob.DownloadFileOptions) (int64, error) {
	if m.ShouldFail {
		return 0, errors.New("download failed")
	}
	_, err := file.Write([]byte("mock blob content"))
	if err != nil {
		return 0, err
	}
	return int64(len("mock blob content")), nil
}
