package cloud

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/SaiNageswarS/go-api-boot/config"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/iterator"
)

/*──────────────────────────────────────────────────────────────────────────────
  Helpers to fabricate a working *secretmanager.SecretIterator via reflection.
  We only need the unexported fields ‘items’ and ‘nextFunc’.
 ────────────────────────────────────────────────────────────────────────────*/

func newSecretIterator(secrets []*secretmanagerpb.Secret) *secretmanager.SecretIterator {
	it := &secretmanager.SecretIterator{}

	// Use reflection/unsafe to poke unexported fields.
	rv := reflect.ValueOf(it).Elem()

	// Set items.
	itemsField := rv.FieldByName("items")
	realItems := reflect.NewAt(itemsField.Type(), unsafe.Pointer(itemsField.UnsafeAddr())).Elem()
	realItems.Set(reflect.ValueOf(secrets))

	// Supply a very small nextFunc: if items empty → iterator.Done
	nextFuncField := rv.FieldByName("nextFunc")
	nextFunc := func() error {
		if realItems.Len() == 0 {
			return iterator.Done
		}
		return nil
	}
	realNext := reflect.NewAt(nextFuncField.Type(), unsafe.Pointer(nextFuncField.UnsafeAddr())).Elem()
	realNext.Set(reflect.ValueOf(nextFunc))

	return it
}

/*
──────────────────────────────────────────────────────────────────────────────

	 stubSecrets – minimal implementation of SecretManagerClient.
	────────────────────────────────────────────────────────────────────────────
*/
type stubSecrets struct {
	secrets []*secretmanagerpb.Secret // listSecrets result
	values  map[string]string         // full-name ➜ value
	failOn  string                    // full-name that should error
}

func (s *stubSecrets) ListSecrets(_ context.Context,
	_ *secretmanagerpb.ListSecretsRequest,
	_ ...gax.CallOption) *secretmanager.SecretIterator {

	return newSecretIterator(append([]*secretmanagerpb.Secret(nil), s.secrets...))
}

func (s *stubSecrets) AccessSecretVersion(_ context.Context,
	req *secretmanagerpb.AccessSecretVersionRequest,
	_ ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error) {

	name := strings.TrimSuffix(req.Name, "/versions/latest")
	if name == s.failOn {
		return nil, context.Canceled
	}
	return &secretmanagerpb.AccessSecretVersionResponse{
		Payload: &secretmanagerpb.SecretPayload{Data: []byte(s.values[name])},
	}, nil
}

/*
──────────────────────────────────────────────────────────────────────────────

	 Utilities to create *GCP instances wired with our stub and with the internal
	 sync.Once already marked “done” so EnsureSecrets() is a no-op.
	────────────────────────────────────────────────────────────────────────────
*/
func newGCPWithStubSecrets(ccfgg *config.BootConfig, stub secretManagerClient) *GCP {
	g := &GCP{ccfgg: ccfgg, Secrets: stub}
	g.secretsOnce.Do(func() {}) // mark Once as executed
	return g
}

/*──────────────────────────────────────────────────────────────────────────────
  Tests
 ────────────────────────────────────────────────────────────────────────────*/

func TestLoadSecretsIntoEnv_HappyPath(t *testing.T) {
	ctx := context.Background()
	const projectID = "unit-proj"
	ccfgg := &config.BootConfig{GcpProjectId: projectID}

	fullA := "projects/" + projectID + "/secrets/API_KEY"
	fullB := "projects/" + projectID + "/secrets/DB_URL"
	stub := &stubSecrets{
		secrets: []*secretmanagerpb.Secret{{Name: fullA}, {Name: fullB}},
		values: map[string]string{
			fullA: "value-A",
			fullB: "value-B",
		},
	}

	g := newGCPWithStubSecrets(ccfgg, stub)
	g.LoadSecretsIntoEnv(ctx)

	assert.Equal(t, os.Getenv("API_KEY"), "value-A", "API_KEY should be set to value-A")
	assert.Equal(t, os.Getenv("DB_URL"), "value-B", "DB_URL should be set to value-B")
}

func TestLoadSecretsIntoEnv_AccessErrorIsIgnored(t *testing.T) {
	ctx := context.Background()
	const projectID = "unit-proj"
	ccfgg := &config.BootConfig{GcpProjectId: projectID}

	fullGood := "projects/" + projectID + "/secrets/GOOD"
	fullBad := "projects/" + projectID + "/secrets/BAD"
	stub := &stubSecrets{
		secrets: []*secretmanagerpb.Secret{{Name: fullGood}, {Name: fullBad}},
		values:  map[string]string{fullGood: "good"},
		failOn:  fullBad,
	}

	g := newGCPWithStubSecrets(ccfgg, stub)
	g.LoadSecretsIntoEnv(ctx)
	assert.Equal(t, os.Getenv("GOOD"), "good", "GOOD should be set to 'good'")
	// BAD should not be set due to the error.
	assert.Empty(t, os.Getenv("BAD"), "BAD should not be set due to access error")
}

func TestLoadSecretsIntoEnv_NoProjectID(t *testing.T) {
	ctx := context.Background()
	ccfgg := &config.BootConfig{GcpProjectId: ""} // ensure absent

	stub := &stubSecrets{} // never reached
	g := newGCPWithStubSecrets(ccfgg, stub)
	err := g.LoadSecretsIntoEnv(ctx)
	assert.Error(t, err, "should return error when GCP project ID is not set")
	assert.EqualError(t, err, "gcp_project_id config is not set", "error message should indicate missing project ID")
}

func TestProvideGCP(t *testing.T) {
	ccfgg := &config.BootConfig{GcpProjectId: "test-project"}
	gcp := ProvideGCP(ccfgg)

	assert.NotNil(t, gcp, "ProvideGCP should return a non-nil GCP instance")
	assert.IsType(t, &GCP{}, gcp, "ProvideGCP should return an instance of GCP")
	assert.Equal(t, ccfgg, gcp.(*GCP).ccfgg, "GCP instance should have the provided config")
}

// ---------- EnsureStorage / EnsureSecrets error propagation -------------
func TestEnsureStorage_Error(t *testing.T) {
	restore := newStorage
	defer func() { newStorage = restore }()

	newStorage = func(ctx context.Context) (storageClient, error) {
		return nil, errors.New("boom")
	}
	gcp := &GCP{}
	err := gcp.EnsureStorage(context.Background())
	assert.EqualError(t, err, "boom")
}

func TestEnsureSecrets_Error(t *testing.T) {
	restore := newSecrets
	defer func() { newSecrets = restore }()

	newSecrets = func(ctx context.Context) (secretManagerClient, error) {
		return nil, errors.New("no-secret")
	}
	gcp := &GCP{}
	err := gcp.EnsureSecrets(context.Background())
	assert.EqualError(t, err, "no-secret")
}
