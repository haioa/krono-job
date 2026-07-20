package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New 返回一个结构化 SugaredLogger，输出到 stdout，便于容器采集。
func New() (*zap.SugaredLogger, error) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	l, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	return l.Sugar(), nil
}
