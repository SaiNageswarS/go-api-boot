package cloud

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/stretchr/testify/assert"
)

func TestAzure_LoadSecretsIntoEnv(t *testing.T) {
	mockSecrets := map[string]string{
		"FOO": "bar",
		"BAZ": "qux",
	}
	collectedEnv := map[string]string{}

	a := &Azure{
		overrideVaultClient: func() (KeyVaultClient, error) {
			return &mockVaultClient{secrets: mockSecrets}, nil
		},
		overrideSetEnv: func(key, value string) error {
			collectedEnv[key] = value
			return nil
		},
	}

	a.LoadSecretsIntoEnv()

	assert.Equal(t, "bar", collectedEnv["FOO"])
	assert.Equal(t, "qux", collectedEnv["BAZ"])
}

func TestAzure_UploadStream_Success(t *testing.T) {
	a := &Azure{
		overrideBlobClient: func(account string) (BlobClient, error) {
			return &mockBlobClient{}, nil
		},
	}

	resultChan, errChan := a.UploadStream("mystorage", "container", "myblob.txt", []byte("test content"))

	select {
	case err := <-errChan:
		t.Fatalf("Unexpected error: %v", err)
	case url := <-resultChan:
		assert.Contains(t, url, "https://mystorage.blob.core.windows.net/container/myblob.txt")
	}
}

func TestAzure_UploadStream_Failure(t *testing.T) {
	a := &Azure{
		overrideBlobClient: func(account string) (BlobClient, error) {
			return &mockBlobClient{ShouldFail: true}, nil
		},
	}

	resultChan, errChan := a.UploadStream("mystorage", "container", "myblob.txt", []byte("test content"))

	select {
	case res := <-resultChan:
		t.Fatalf("Expected error but got result: %v", res)
	case err := <-errChan:
		assert.EqualError(t, err, "upload failed")
	}
}

func TestAzure_UploadStream_BlobClientNil(t *testing.T) {
	a := &Azure{
		overrideBlobClient: func(account string) (BlobClient, error) {
			return nil, errors.New("simulated init failure")
		},
	}

	resultChan, errChan := a.UploadStream("badaccount", "container", "myblob.txt", []byte("data"))

	select {
	case <-resultChan:
		t.Fatal("Expected error, got success")
	case err := <-errChan:
		assert.ErrorContains(t, err, "simulated init failure")
	}
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
