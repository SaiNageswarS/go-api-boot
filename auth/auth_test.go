package auth

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Test generate token and verify same token success test.
func TestGenerateAndVerifyToken(t *testing.T) {
	// Set ACCESS_SECRET environment variable
	os.Setenv("ACCESS-SECRET", "CONST-SECRET")
	// Generate token
	token, _ := GetToken("testTenant", "rick", "non-admin")
	// Verify token
	userId, tenant, userType, err := decryptToken(token)

	assert.NoError(t, err)
	assert.Equal(t, "rick", userId)
	assert.Equal(t, "testTenant", tenant)
	assert.Equal(t, "non-admin", userType)
}

func TestGenerateAccessSecretNotSet(t *testing.T) {
	// Clear ACCESS_SECRET environment variable
	os.Unsetenv("ACCESS-SECRET")
	// Generate token
	token, err := GetToken("testTenant", "rick", "non-admin")
	assert.Error(t, err)
	assert.Empty(t, token)
}

func TestFailTokenTampered(t *testing.T) {
	// Set ACCESS_SECRET environment variable
	os.Setenv("ACCESS-SECRET", "CONST-SECRET")
	// Generate token
	token, _ := GetToken("testTenant", "rick", "non-admin")
	// Tamper token
	token = token + "tampered"
	// Verify token
	_, _, _, err := decryptToken(token)
	assert.Error(t, err)
}

func TestFailAccessSecretChanged(t *testing.T) {
	// Set ACCESS_SECRET environment variable
	os.Setenv("ACCESS-SECRET", "FIRST-SECRET")
	// Generate token
	token, _ := GetToken("testTenant", "rick", "non-admin")
	// Set ACCESS_SECRET environment variable
	os.Setenv("ACCESS-SECRET", "SECOND-SECRET")
	// Verify token
	_, _, _, err := decryptToken(token)
	assert.Error(t, err)
}

func TestReadClaimsFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), USER_ID_CLAIM, "rick")
	ctx = context.WithValue(ctx, TENANT_CLAIM, "testTenant")
	ctx = context.WithValue(ctx, USER_TYPE_CLAIM, "non-admin")

	userId, tenant := GetUserIdAndTenant(ctx)

	assert.Equal(t, "rick", userId)
	assert.Equal(t, "testTenant", tenant)

	userType := GetUserType(ctx)
	assert.Equal(t, "non-admin", userType)
}

func TestVerifyToken_NoAuthHeader(t *testing.T) {
	f := VerifyToken()
	_, err := f(context.Background())
	assert.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestVerifyToken_DecryptionError(t *testing.T) {
	restore := decryptToken
	defer func() { decryptToken = restore }()

	decryptToken = func(string) (string, string, string, error) {
		return "", "", "", errors.New("bad-token")
	}

	md := metadata.Pairs("authorization", "Bearer abc.def.ghi")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	f := VerifyToken()
	_, err := f(ctx)
	assert.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestVerifyToken_Success(t *testing.T) {
	restore := decryptToken
	defer func() { decryptToken = restore }()

	// stub returns deterministic claims
	decryptToken = func(string) (string, string, string, error) {
		return "u123", "acme", "admin", nil
	}

	md := metadata.Pairs("authorization", "Bearer valid.jwt")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	f := VerifyToken()
	newCtx, err := f(ctx)
	assert.NoError(t, err)

	assert.Equal(t, "u123", newCtx.Value(USER_ID_CLAIM))
	assert.Equal(t, "acme", newCtx.Value(TENANT_CLAIM))
	assert.Equal(t, "admin", newCtx.Value(USER_TYPE_CLAIM))
}
