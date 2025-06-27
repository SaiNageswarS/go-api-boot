package logger

import (
	"os"

	"go.uber.org/zap"
)

var Log *zap.Logger = getLogger()

func getLogger() *zap.Logger {
	runMode := os.Getenv("ENV")

	var log *zap.Logger
	var err error

	switch runMode {
	case "prod":
		log, err = zap.NewProduction()
	default:
		log, err = zap.NewDevelopment()
	}

	if err != nil {
		log.Panic("Unable to get zapper")
	}
	return log
}

func Get() *zap.Logger {
	return Log
}

var Info = func(msg string, fields ...zap.Field) {
	Log.Info(msg, fields...)
}

var Fatal = func(msg string, fields ...zap.Field) {
	Log.Fatal(msg, fields...)
}

var Error = func(msg string, fields ...zap.Field) {
	Log.Error(msg, fields...)
}

var Debug = func(msg string, fields ...zap.Field) {
	Log.Debug(msg, fields...)
}
