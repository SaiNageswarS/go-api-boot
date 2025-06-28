package logger

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestGetLogger_Development(t *testing.T) {
	t.Setenv("ENV", "") // anything except "prod"
	l := getLogger()

	assert.True(t, l.Core().Enabled(zap.DebugLevel),
		"development logger should allow Debug level")
	assert.True(t, l.Core().Enabled(zap.InfoLevel),
		"development logger should allow Info level")
}

func TestGetLogger_Production(t *testing.T) {
	t.Setenv("ENV", "prod")
	l := getLogger()

	assert.False(t, l.Core().Enabled(zap.DebugLevel),
		"production logger should NOT allow Debug level by default")
	assert.True(t, l.Core().Enabled(zap.InfoLevel),
		"production logger should allow Info level")
}

// ---- convenience wrappers ---------------------------------------------------

func TestWrappers_LogThroughGlobal(t *testing.T) {
	// Capture all logs at Debug level.
	core, rec := observer.New(zap.DebugLevel)
	testLog := zap.New(core)

	// Swap the global temporarily.
	orig := Log
	Log = testLog
	defer func() { Log = orig }()

	Info("info message")
	Debug("debug message")
	Error("error message")

	waitForLogs(t, rec, 3)

	entries := rec.All()
	assert.Equal(t, 3, len(entries))
	assert.Equal(t, zap.InfoLevel, entries[0].Level)
	assert.Equal(t, "info message", entries[0].Message)

	assert.Equal(t, zap.DebugLevel, entries[1].Level)
	assert.Equal(t, "debug message", entries[1].Message)

	assert.Equal(t, zap.ErrorLevel, entries[2].Level)
	assert.Equal(t, "error message", entries[2].Message)
}

// helper to wait until a log entry appears or timeout (useful on slow CI)
func waitForLogs(t *testing.T, rec *observer.ObservedLogs, want int) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	for {
		if rec.Len() >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("expected %d log entries, got %d", want, rec.Len())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
