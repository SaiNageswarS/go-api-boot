package bootUtils

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetEnvOrDefault(t *testing.T) {
	key := "TEST_ENV_VAR"

	t.Run("EnvironmentVariableSet", func(t *testing.T) {
		expectedValue := "value_from_env"
		os.Setenv(key, expectedValue)
		defer os.Unsetenv(key) // Ensure to clean up the environment variable after the test

		result := GetEnvOrDefault(key, "default_value")
		require.Equal(t, expectedValue, result)
	})

	t.Run("EnvironmentVariableNotSet", func(t *testing.T) {
		os.Unsetenv(key) // Ensure the environment variable is not set

		expectedFallback := "default_value"
		result := GetEnvOrDefault(key, expectedFallback)
		require.Equal(t, expectedFallback, result)
	})

	t.Run("EmptyEnvironmentVariable", func(t *testing.T) {
		expectedValue := ""
		os.Setenv(key, expectedValue)
		defer os.Unsetenv(key) // Ensure to clean up the environment variable after the test

		result := GetEnvOrDefault(key, "default_value")
		require.Equal(t, expectedValue, result)
	})

	t.Run("EmptyFallback", func(t *testing.T) {
		os.Unsetenv(key) // Ensure the environment variable is not set

		expectedFallback := ""
		result := GetEnvOrDefault(key, expectedFallback)
		require.Equal(t, expectedFallback, result)
	})
}

func mockFunction(attemptsBeforeSuccess int) func() error {
	attempts := 0
	return func() error {
		attempts++
		if attempts < attemptsBeforeSuccess {
			return errors.New("not yet")
		}
		return nil
	}
}

func TestRetryWithExponentialBackoff_Success(t *testing.T) {
	baseDelay := 10 * time.Millisecond
	maxRetries := 5

	// Test a case where the function succeeds within the retry limit
	fn := mockFunction(3) // Succeeds on the 3rd attempt
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := RetryWithExponentialBackoff(ctx, maxRetries, baseDelay, fn)
	require.NoError(t, err)
}

func TestRetryWithExponentialBackoff_ImmediateSuccess(t *testing.T) {
	baseDelay := 10 * time.Millisecond
	maxRetries := 5

	// Test a case where the function succeeds immediately
	fn := mockFunction(1) // Succeeds on the 1st attempt
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := RetryWithExponentialBackoff(ctx, maxRetries, baseDelay, fn)
	require.NoError(t, err)
}
