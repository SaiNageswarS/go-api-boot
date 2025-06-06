package cloud

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
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

func TestAzure_UploadBuffer_Success(t *testing.T) {
	config := &config.BootConfig{
		AzureStorageAccount: "mystorage",
	}

	a := &Azure{
		ccfgg:      config,
		BlobClient: &mockBlobClient{},
	}

	url, err := a.UploadBuffer(context.Background(), "container", "myblob.txt", []byte("test content"))

	assert.NoError(t, err)
	assert.Contains(t, url, "https://mystorage.blob.core.windows.net/container/myblob.txt")
}

func TestAzure_UploadBuffer_Failure(t *testing.T) {
	config := &config.BootConfig{
		AzureStorageAccount: "mystorage",
	}

	a := &Azure{
		ccfgg:      config,
		BlobClient: &mockBlobClient{ShouldFail: true},
	}

	url, err := a.UploadBuffer(context.Background(), "container", "myblob.txt", []byte("test content"))

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

func TestAzure_ProvideAzure(t *testing.T) {
	config := &config.BootConfig{
		AzureStorageAccount: "mystorage",
	}

	c := ProvideAzure(config)
	assert.NotNil(t, c)

	// assert a is of type *Azure
	az, ok := c.(*Azure)
	assert.True(t, ok)

	// clients are nil and should be lazily initialized
	assert.Nil(t, az.KvClient)
	assert.Nil(t, az.BlobClient)
}

func TestGetKeyvaultClient_MissingName(t *testing.T) {
	_, err := getKeyvaultClient(&config.BootConfig{AzureKeyVaultName: ""})
	assert.Error(t, err)
	assert.EqualError(t, err, "azure_key_vault_name config not set")
}

func TestGetKeyvaultClient_CredentialFail(t *testing.T) {
	restore := newDefaultCred
	defer func() { newDefaultCred = restore }()

	newDefaultCred = func() (*azidentity.DefaultAzureCredential, error) {
		return nil, errors.New("cred fail")
	}

	_, err := getKeyvaultClient(&config.BootConfig{AzureKeyVaultName: "kv"})
	assert.Error(t, err)
	assert.EqualError(t, err, "cred fail")
}

func TestGetKeyvaultClient_ClientFail(t *testing.T) {
	restoreCred, restoreKV := newDefaultCred, newKVClient
	defer func() { newDefaultCred, newKVClient = restoreCred, restoreKV }()

	newDefaultCred = func() (*azidentity.DefaultAzureCredential, error) { return &azidentity.DefaultAzureCredential{}, nil }
	newKVClient = func(url string, _ *azidentity.DefaultAzureCredential) (keyVaultClient, error) {
		return nil, errors.New("kv client fail")
	}

	_, err := getKeyvaultClient(&config.BootConfig{AzureKeyVaultName: "kv"})
	assert.Error(t, err)
	assert.EqualError(t, err, "kv client fail")
}

func TestGetKeyvaultClient_OK(t *testing.T) {
	restoreCred, restoreKV := newDefaultCred, newKVClient
	defer func() { newDefaultCred, newKVClient = restoreCred, restoreKV }()

	newDefaultCred = func() (*azidentity.DefaultAzureCredential, error) { return &azidentity.DefaultAzureCredential{}, nil }
	want := &mockVaultClient{}
	newKVClient = func(url string, _ *azidentity.DefaultAzureCredential) (keyVaultClient, error) {
		assert.NotEmpty(t, url, "URL should not be empty")
		assert.Equal(t, "https://my.vault.azure.net/", url, "URL should match expected format")
		return want, nil
	}

	got, err := getKeyvaultClient(&config.BootConfig{AzureKeyVaultName: "my"})
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGetBlob_CredentialFail(t *testing.T) {
	restore := newDefaultCred
	defer func() { newDefaultCred = restore }()
	newDefaultCred = func() (*azidentity.DefaultAzureCredential, error) { return nil, errors.New("cred oops") }

	_, err := getServiceClientTokenCredential("acct")
	assert.Error(t, err)
	assert.EqualError(t, err, "cred oops")
}

func TestGetBlob_ClientFail(t *testing.T) {
	restoreCred, restoreBlob := newDefaultCred, newBlobClient
	defer func() { newDefaultCred, newBlobClient = restoreCred, restoreBlob }()

	newDefaultCred = func() (*azidentity.DefaultAzureCredential, error) { return &azidentity.DefaultAzureCredential{}, nil }
	newBlobClient = func(url string, _ *azidentity.DefaultAzureCredential) (blobClient, error) {
		return nil, errors.New("blob oops")
	}

	_, err := getServiceClientTokenCredential("acct")
	assert.NotNil(t, err)
	assert.EqualError(t, err, "blob oops")
}

func TestGetBlob_OK(t *testing.T) {
	restoreCred, restoreBlob := newDefaultCred, newBlobClient
	defer func() { newDefaultCred, newBlobClient = restoreCred, restoreBlob }()

	newDefaultCred = func() (*azidentity.DefaultAzureCredential, error) { return &azidentity.DefaultAzureCredential{}, nil }
	want := &mockBlobClient{}
	newBlobClient = func(url string, _ *azidentity.DefaultAzureCredential) (blobClient, error) {
		assert.Equal(t, "https://acct.blob.core.windows.net/", url, "URL should match expected format")
		return want, nil
	}
	got, err := getServiceClientTokenCredential("acct")
	assert.NoError(t, err)
	assert.Equal(t, want, got)
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
