package auth

import (
	"context"
	"errors"
	"os"

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

func VerifyToken() grpc_auth.AuthFunc {
	return func(ctx context.Context) (context.Context, error) {
		token, err := grpc_auth.AuthFromMD(ctx, "bearer")
		if err != nil {
			logger.Error("Error getting token", zap.Error(err))
			return nil, status.Errorf(codes.Unauthenticated, err.Error())
		}

		userId, tenant, userType, err := decryptToken(token)
		if err != nil {
			logger.Error("Error getting token", zap.Error(err))
			return nil, status.Errorf(codes.Unauthenticated, err.Error())
		}

		newCtx := context.WithValue(ctx, USER_ID_CLAIM, userId)
		newCtx = context.WithValue(newCtx, TENANT_CLAIM, tenant)
		newCtx = context.WithValue(newCtx, USER_TYPE_CLAIM, userType)

		return newCtx, nil
	}
}

func GetToken(tenant, userId, userType string) string {
	atClaims := jwt.StandardClaims{}
	atClaims.Id = userId
	atClaims.Audience = tenant
	atClaims.Subject = userType

	var ACCESS_SECRET = os.Getenv("ACCESS-SECRET")
	at := jwt.NewWithClaims(jwt.SigningMethodHS256, atClaims)
	token, _ := at.SignedString([]byte(ACCESS_SECRET))
	return token
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
func decryptToken(token string) (string, string, string, error) {
	accessSecret := os.Getenv("ACCESS-SECRET")

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
