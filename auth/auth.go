package auth

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/dgrijalva/jwt-go"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Claims string

var USER_ID_CLAIM = Claims("userId")
var TENANT_CLAIM = Claims("tenantId")
var USER_TYPE_CLAIM = Claims("userType")

func VerifyTokenGrpcMiddleware() grpc_auth.AuthFunc {
	return func(ctx context.Context) (context.Context, error) {
		token, err := grpc_auth.AuthFromMD(ctx, "bearer")
		if err != nil {
			logger.Error("Error getting token", zap.Error(err))
			return nil, status.Error(codes.Unauthenticated, "missing or malformed token")
		}

		userId, tenant, userType, err := decryptToken(token)
		if err != nil {
			logger.Error("Error getting token", zap.Error(err))
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		newCtx := context.WithValue(ctx, USER_ID_CLAIM, userId)
		newCtx = context.WithValue(newCtx, TENANT_CLAIM, tenant)
		newCtx = context.WithValue(newCtx, USER_TYPE_CLAIM, userType)

		return newCtx, nil
	}
}

func VerifyTokenHttpMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			logger.Error("Missing Authorization header")
			http.Error(w, "missing or malformed token", http.StatusUnauthorized)
			return
		}

		splits := strings.SplitN(authHeader, " ", 2)

		// Check for Bearer scheme (case-insensitive)
		if len(splits) < 2 || !strings.EqualFold(splits[0], "bearer") {
			logger.Error("Bad authorization string")
			http.Error(w, "missing or malformed token", http.StatusUnauthorized)
			return
		}

		token := splits[1]

		userId, tenant, userType, err := decryptToken(token)
		if err != nil {
			logger.Error("Error decrypting token", zap.Error(err))
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), USER_ID_CLAIM, userId)
		ctx = context.WithValue(ctx, TENANT_CLAIM, tenant)
		ctx = context.WithValue(ctx, USER_TYPE_CLAIM, userType)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetToken(tenant, userId, userType string) (string, error) {
	atClaims := jwt.StandardClaims{}
	atClaims.Id = userId
	atClaims.Audience = tenant
	atClaims.Subject = userType

	var ACCESS_SECRET = os.Getenv("ACCESS-SECRET")
	if ACCESS_SECRET == "" {
		return "", errors.New("ACCESS-SECRET is not set in environment")
	}

	at := jwt.NewWithClaims(jwt.SigningMethodHS256, atClaims)
	token, err := at.SignedString([]byte(ACCESS_SECRET))

	if err != nil {
		logger.Error("Error signing token", zap.Error(err))
		return "", err
	}
	return token, nil
}

func GetUserIdAndTenant(ctx context.Context) (string, string) {
	userIdClaim := ctx.Value(USER_ID_CLAIM)
	tenantClaim := ctx.Value(TENANT_CLAIM)

	var userId, tenant string

	if userIdClaimStr, ok := userIdClaim.(string); ok {
		userId = userIdClaimStr
	}

	if tenantClaimStr, ok := tenantClaim.(string); ok {
		tenant = tenantClaimStr
	}

	return userId, tenant
}

func GetUserType(ctx context.Context) string {
	userTypeClaim := ctx.Value(USER_TYPE_CLAIM)
	if userTypeClaimStr, ok := userTypeClaim.(string); ok {
		return userTypeClaimStr
	}

	return ""
}

// returns userId, tenant, userType
var decryptToken = func(token string) (string, string, string, error) {
	accessSecret := os.Getenv("ACCESS-SECRET")
	if accessSecret == "" {
		return "", "", "", errors.New("ACCESS-SECRET is not set in environment")
	}

	parsedToken, err := jwt.ParseWithClaims(
		token,
		&jwt.StandardClaims{},
		func(token *jwt.Token) (interface{}, error) {
			return []byte(accessSecret), nil
		})

	if err != nil {
		return "", "", "", err
	}

	claims, ok := parsedToken.Claims.(*jwt.StandardClaims)

	if !ok || !parsedToken.Valid {
		return "", "", "", errors.New("failed reading claims")
	}

	return claims.Id, claims.Audience, claims.Subject, nil
}
