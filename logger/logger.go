package logger

import "go.uber.org/zap"

var Log *zap.Logger = getLogger()

func getLogger() *zap.Logger {
	log, err := zap.NewDevelopment()

	if err != nil {
		log.Panic("Unable to get zapper")
	}
	return log
}

func Get() *zap.Logger {
	return Log
}

func Info(msg string, fields ...zap.Field) {
	Log.Info(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	Log.Fatal(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	Log.Error(msg, fields...)
}
