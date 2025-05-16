package cloud

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	azsecrets "github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestLoadSecretsIntoEnv(t *testing.T) {
	secrets := map[string]string{
		"DB_USER": "admin",
		"DB_PASS": "secret",
	}
	az := &Azure{KeyVaultClient: &mockKeyVaultClient{Secrets: secrets}}
	az.LoadSecretsIntoEnv()

	assert.Equal(t, "admin", os.Getenv("DB_USER"))
	assert.Equal(t, "secret", os.Getenv("DB_PASS"))
}

func TestUploadStreamSuccess(t *testing.T) {
	az := &Azure{StorageClient: &mockUploader{}}
	resCh, errCh := az.UploadStream("test-container", "path.txt", *bytes.NewBufferString("data"))

	select {
	case url := <-resCh:
		assert.Contains(t, url, "https://mock.blob.core.windows.net/test-container/path.txt")
	case err := <-errCh:
		t.Errorf("Unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for result")
	}
}

func TestUploadStreamFailure(t *testing.T) {
	az := &Azure{StorageClient: &mockUploader{Fail: true}}
	_, errCh := az.UploadStream("test-container", "path.txt", *bytes.NewBufferString("data"))

	select {
	case err := <-errCh:
		assert.EqualError(t, err, "upload failed")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for error")
	}
}

type mockKeyVaultClient struct {
	mock.Mock
	Secrets map[string]string
}

func (m *mockKeyVaultClient) ListSecrets(opts *azsecrets.ListSecretsOptions) *mockPager {
	return &mockPager{secrets: m.Secrets}
}

func (m *mockKeyVaultClient) GetSecret(ctx context.Context, name, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
	val, ok := m.Secrets[name]
	if !ok {
		return azsecrets.GetSecretResponse{}, errors.New("not found")
	}
	return azsecrets.GetSecretResponse{Value: &val}, nil
}

type mockPager struct {
	secrets map[string]string
	called  bool
}

func (m *mockPager) More() bool {
	return !m.called
}

func (m *mockPager) NextPage(ctx context.Context) (azsecrets.ListSecretsResponse, error) {
	m.called = true
	var props []*azsecrets.SecretProperties
	for k := range m.secrets {
		name := k
		props = append(props, &azsecrets.SecretProperties{ID: &azsecrets.ID{Name: &name}})
	}
	return azsecrets.ListSecretsResponse{Value: props}, nil
}

type mockUploader struct {
	Fail bool
}

func (m *mockUploader) Upload(container, path string, data []byte) (string, error) {
	if m.Fail {
		return "", errors.New("upload failed")
	}
	return "https://mock.blob.core.windows.net/" + container + "/" + path, nil
}
