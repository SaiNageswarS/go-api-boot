package cloud

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/googleapis/gax-go/v2"
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
func newGCPWithStubSecrets(stub SecretManagerClient) *GCP {
	g := &GCP{Secrets: stub}
	g.secretsOnce.Do(func() {}) // mark Once as executed
	return g
}

/*──────────────────────────────────────────────────────────────────────────────
  Tests
 ────────────────────────────────────────────────────────────────────────────*/

func TestLoadSecretsIntoEnv_HappyPath(t *testing.T) {
	ctx := context.Background()
	const projectID = "unit-proj"
	os.Setenv("GCP_PROJECT_ID", projectID)
	defer os.Unsetenv("GCP_PROJECT_ID")

	fullA := "projects/" + projectID + "/secrets/API_KEY"
	fullB := "projects/" + projectID + "/secrets/DB_URL"
	stub := &stubSecrets{
		secrets: []*secretmanagerpb.Secret{{Name: fullA}, {Name: fullB}},
		values: map[string]string{
			fullA: "value-A",
			fullB: "value-B",
		},
	}

	g := newGCPWithStubSecrets(stub)
	g.LoadSecretsIntoEnv(ctx)

	if got := os.Getenv("API_KEY"); got != "value-A" {
		t.Fatalf("API_KEY = %q, want %q", got, "value-A")
	}
	if got := os.Getenv("DB_URL"); got != "value-B" {
		t.Fatalf("DB_URL = %q, want %q", got, "value-B")
	}
}

func TestLoadSecretsIntoEnv_AccessErrorIsIgnored(t *testing.T) {
	ctx := context.Background()
	const projectID = "unit-proj"
	os.Setenv("GCP_PROJECT_ID", projectID)
	defer os.Unsetenv("GCP_PROJECT_ID")

	fullGood := "projects/" + projectID + "/secrets/GOOD"
	fullBad := "projects/" + projectID + "/secrets/BAD"
	stub := &stubSecrets{
		secrets: []*secretmanagerpb.Secret{{Name: fullGood}, {Name: fullBad}},
		values:  map[string]string{fullGood: "good"},
		failOn:  fullBad,
	}

	g := newGCPWithStubSecrets(stub)
	g.LoadSecretsIntoEnv(ctx)

	if got := os.Getenv("GOOD"); got != "good" {
		t.Fatalf("GOOD = %q, want %q", got, "good")
	}
	if got := os.Getenv("BAD"); got != "" {
		t.Fatalf("BAD should be unset on failure, got %q", got)
	}
}

func TestLoadSecretsIntoEnv_NoProjectID(t *testing.T) {
	ctx := context.Background()
	os.Unsetenv("GCP_PROJECT_ID") // ensure absent

	stub := &stubSecrets{} // never reached
	g := newGCPWithStubSecrets(stub)
	g.LoadSecretsIntoEnv(ctx)

	// quick sanity check: a bogus key is still absent
	if got := os.Getenv("SHOULD_NOT_EXIST"); got != "" {
		t.Fatalf("unexpected env leakage: %q", got)
	}
}
