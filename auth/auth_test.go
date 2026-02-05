package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/testutil"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Test generate token and verify same token success test.
func TestGenerateAndVerifyToken(t *testing.T) {
	testutil.WithEnv("ACCESS-SECRET", "CONST-SECRET", func(logger *testutil.MockLogger) {
		// Generate token
		token, _ := GetToken("testTenant", "rick", "non-admin")
		// Verify token
		userId, tenant, userType, err := decryptToken(token)

		assert.NoError(t, err)
		assert.Equal(t, "rick", userId)
		assert.Equal(t, "testTenant", tenant)
		assert.Equal(t, "non-admin", userType)
	})
}

func TestGenerateAccessSecretNotSet(t *testing.T) {
	testutil.WithEnv("ACCESS-SECRET", "", func(logger *testutil.MockLogger) {
		// Generate token
		token, err := GetToken("testTenant", "rick", "non-admin")
		assert.Error(t, err)
		assert.Empty(t, token)
	})
}

func TestFailTokenTampered(t *testing.T) {
	testutil.WithEnv("ACCESS-SECRET", "CONST-SECRET", func(logger *testutil.MockLogger) {
		// Generate token
		token, _ := GetToken("testTenant", "rick", "non-admin")
		// Tamper token
		token = token + "tampered"
		// Verify token
		_, _, _, err := decryptToken(token)
		assert.Error(t, err)
	})
}

func TestFailAccessSecretChanged(t *testing.T) {
	testutil.WithEnv("ACCESS-SECRET", "FIRST-SECRET", func(logger *testutil.MockLogger) {
		// Generate token
		token, _ := GetToken("testTenant", "rick", "non-admin")
		// Set ACCESS_SECRET environment variable
		os.Setenv("ACCESS-SECRET", "SECOND-SECRET")
		// Verify token
		_, _, _, err := decryptToken(token)
		assert.Error(t, err)
	})
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
	f := VerifyTokenGrpcMiddleware()
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

	f := VerifyTokenGrpcMiddleware()
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

	f := VerifyTokenGrpcMiddleware()
	newCtx, err := f(ctx)
	assert.NoError(t, err)

	assert.Equal(t, "u123", newCtx.Value(USER_ID_CLAIM))
	assert.Equal(t, "acme", newCtx.Value(TENANT_CLAIM))
	assert.Equal(t, "admin", newCtx.Value(USER_TYPE_CLAIM))
}

func TestVerifyTokenHttpMiddleware_NoAuthHeader(t *testing.T) {
	// Create a test handler that should not be called
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when auth header is missing")
	})

	// Create middleware
	middleware := VerifyTokenHttpMiddleware(testHandler)

	// Create request without Authorization header
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	recorder := httptest.NewRecorder()

	// Execute middleware
	middleware.ServeHTTP(recorder, req)

	// Verify response
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "missing or malformed token")
}

func TestVerifyTokenHttpMiddleware_MalformedAuthHeader(t *testing.T) {
	// Create a test handler that should not be called
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when auth header is malformed")
	})

	// Create middleware
	middleware := VerifyTokenHttpMiddleware(testHandler)

	// Test cases for malformed headers
	testCases := []struct {
		name   string
		header string
	}{
		{"No Bearer prefix", "token123"},
		{"Missing token", "Bearer"},
		{"Wrong scheme", "Basic token123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", tc.header)
			recorder := httptest.NewRecorder()

			middleware.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "missing or malformed token")
		})
	}
}

func TestVerifyTokenHttpMiddleware_EmptyBearerToken(t *testing.T) {
	// Create a test handler that should not be called
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when bearer token is empty")
	})

	// Create middleware
	middleware := VerifyTokenHttpMiddleware(testHandler)

	// Create request with empty bearer token
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	recorder := httptest.NewRecorder()

	// Execute middleware
	middleware.ServeHTTP(recorder, req)

	// Verify response (empty token will cause decryption error)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid token")
}

func TestVerifyTokenHttpMiddleware_DecryptionError(t *testing.T) {
	restore := decryptToken
	defer func() { decryptToken = restore }()

	// Stub decryptToken to return error
	decryptToken = func(string) (string, string, string, error) {
		return "", "", "", errors.New("invalid token")
	}

	// Create a test handler that should not be called
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when token decryption fails")
	})

	// Create middleware
	middleware := VerifyTokenHttpMiddleware(testHandler)

	// Create request with valid Authorization header format but invalid token
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	recorder := httptest.NewRecorder()

	// Execute middleware
	middleware.ServeHTTP(recorder, req)

	// Verify response
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "invalid token")
}

func TestVerifyTokenHttpMiddleware_Success(t *testing.T) {
	restore := decryptToken
	defer func() { decryptToken = restore }()

	// Stub decryptToken to return successful claims
	decryptToken = func(string) (string, string, string, error) {
		return "user456", "testcorp", "admin", nil
	}

	// Create a test handler that captures the context values
	var receivedUserId, receivedTenant, receivedUserType string
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if userId, ok := ctx.Value(USER_ID_CLAIM).(string); ok {
			receivedUserId = userId
		}
		if tenant, ok := ctx.Value(TENANT_CLAIM).(string); ok {
			receivedTenant = tenant
		}
		if userType, ok := ctx.Value(USER_TYPE_CLAIM).(string); ok {
			receivedUserType = userType
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Create middleware
	middleware := VerifyTokenHttpMiddleware(testHandler)

	// Create request with valid Authorization header
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer valid.jwt.token")
	recorder := httptest.NewRecorder()

	// Execute middleware
	middleware.ServeHTTP(recorder, req)

	// Verify response
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", recorder.Body.String())

	// Verify context values were set correctly
	assert.Equal(t, "user456", receivedUserId)
	assert.Equal(t, "testcorp", receivedTenant)
	assert.Equal(t, "admin", receivedUserType)
}

func TestVerifyTokenHttpMiddleware_BearerCaseInsensitive(t *testing.T) {
	restore := decryptToken
	defer func() { decryptToken = restore }()

	// Stub decryptToken to return successful claims
	decryptToken = func(string) (string, string, string, error) {
		return "user789", "example", "user", nil
	}

	// Create a test handler
	var handlerCalled bool
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware
	middleware := VerifyTokenHttpMiddleware(testHandler)

	// Test different cases of "Bearer"
	testCases := []string{"Bearer", "bearer", "BEARER", "BeArEr"}

	for _, bearerCase := range testCases {
		t.Run("Bearer_"+bearerCase, func(t *testing.T) {
			handlerCalled = false
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", bearerCase+" valid.token")
			recorder := httptest.NewRecorder()

			middleware.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.True(t, handlerCalled, "Handler should be called for case: "+bearerCase)
		})
	}
}
