package embed

import (
	"os"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
)

func withEnv(key, value string, fn func(logger *MockLogger)) {
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

type MockLogger struct {
	isFatalCalled bool
	fatalMsg      string
}

func (m *MockLogger) Fatal(msg string, fields ...zap.Field) {
	m.isFatalCalled = true
	m.fatalMsg = msg
}
