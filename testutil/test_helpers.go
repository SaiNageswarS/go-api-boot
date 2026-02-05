package testutil

import (
	"os"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
)

// WithEnv temporarily sets an environment variable for the duration of a test function.
// It automatically restores the original value and sets up a mock logger.
func WithEnv(key, value string, fn func(logger *MockLogger)) {
	originalEnv := os.Getenv(key)
	os.Setenv(key, value)
	defer os.Setenv(key, originalEnv)

	mockLogger := &MockLogger{}
	originalLogger := logger.Fatal
	logger.Fatal = mockLogger.Fatal
	defer func() {
		logger.Fatal = originalLogger
	}()

	fn(mockLogger)
}

// MockLogger is a test double for the logger that captures Fatal calls.
type MockLogger struct {
	IsFatalCalled bool
	FatalMsg      string
}

// Fatal implements the logger.Fatal interface for testing purposes.
func (m *MockLogger) Fatal(msg string, fields ...zap.Field) {
	m.IsFatalCalled = true
	m.FatalMsg = msg
}
