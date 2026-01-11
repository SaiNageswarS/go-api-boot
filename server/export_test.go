package server

import (
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
)

// withMockLogger replaces logger.Fatal with a mock that records calls and panics
// (simulating program termination). Returns the mock logger to check assertions
// after the function returns.
func withMockLogger(fn func()) (mockLogger *MockLogger) {
	mockLogger = &MockLogger{}
	originalLogger := logger.Fatal
	logger.Fatal = mockLogger.Fatal
	defer func() {
		logger.Fatal = originalLogger
		// Recover from the panic caused by mock Fatal
		recover()
	}()

	fn()
	return mockLogger
}

type MockLogger struct {
	isFatalCalled bool
	fatalMsg      string
}

func (m *MockLogger) Fatal(msg string, fields ...zap.Field) {
	m.isFatalCalled = true
	m.fatalMsg = msg
	// Panic to simulate logger.Fatal terminating the program
	panic("logger.Fatal called")
}
