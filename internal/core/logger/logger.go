package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.Logger
}

func NewLogger() (*Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "time"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	z, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	return &Logger{z}, nil
}

func (l *Logger) Close() error {
	return l.Sync()
}
