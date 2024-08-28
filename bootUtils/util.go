package bootUtils

import (
	"context"
	"errors"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
	"os"
	"time"

	"golang.org/x/time/rate"
)

func GetEnvOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func RetryWithExponentialBackoff(ctx context.Context, maxRetries int, baseDelay time.Duration, fn func() error) error {
	limiter := rate.NewLimiter(rate.Every(baseDelay), 1)
	retries := 0

	for retries < maxRetries {
		// Wait for the next retry attempt
		if err := limiter.Wait(ctx); err != nil {
			return err
		}

		// Attempt the operation
		if err := fn(); err != nil {
			logger.Error("Failed attempt. ", zap.Int("Try", retries+1), zap.Error(err))
			retries++
			// Increase delay exponentially
			limiter.SetLimit(rate.Every(baseDelay * time.Duration(1<<retries)))
		} else {
			logger.Info("Succeeded")
			return nil
		}
	}

	return errors.New("all attempts failed")
}
