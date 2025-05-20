package auth

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test generate token and verify same token success test.
func TestGenerateAndVerifyToken(t *testing.T) {
	// Set ACCESS_SECRET environment variable
	os.Setenv("ACCESS-SECRET", "CONST-SECRET")
	// Generate token
	token, _ := GetToken("testTenant", "rick", "non-admin")
	// Verify token
	userId, tenant, userType, err := decryptToken(token)

	require.NoError(t, err)
	require.Equal(t, "rick", userId)
	require.Equal(t, "testTenant", tenant)
	require.Equal(t, "non-admin", userType)
}

func TestGenerateAccessSecretNotSet(t *testing.T) {
	// Clear ACCESS_SECRET environment variable
	os.Unsetenv("ACCESS-SECRET")
	// Generate token
	token, err := GetToken("testTenant", "rick", "non-admin")
	require.Error(t, err)
	require.Empty(t, token)
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
	require.Error(t, err)
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
	require.Error(t, err)
}

func TestReadClaimsFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), USER_ID_CLAIM, "rick")
	ctx = context.WithValue(ctx, TENANT_CLAIM, "testTenant")
	ctx = context.WithValue(ctx, USER_TYPE_CLAIM, "non-admin")

	userId, tenant := GetUserIdAndTenant(ctx)

	require.Equal(t, "rick", userId)
	require.Equal(t, "testTenant", tenant)

	userType := GetUserType(ctx)
	require.Equal(t, "non-admin", userType)
}
