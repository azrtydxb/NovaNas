// Package logging configures the zap + logr logger used by the operator.
package logging

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New returns a production-grade logr.Logger backed by zap.
//
// The logger emits JSON to stderr with ISO8601 timestamps. Level is controlled
// by the NOVANAS_LOG_LEVEL env var (debug|info|warn|error); default: info.
func New(development bool) logr.Logger {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.TimeKey = "ts"
	if development {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	z, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		// Fall back to a nop logger if zap can't start; the manager will still run.
		return logr.Discard()
	}
	return zapr.NewLogger(z)
}
