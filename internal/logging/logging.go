// Package logging builds structured local loggers for the engine.
package logging

import (
	"fmt"

	"digital-exhaust-cleaner/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a Zap logger from application logging configuration.
func New(cfg config.LoggingConfig) (*zap.Logger, error) {
	zapCfg := zap.NewProductionConfig()
	if cfg.Development {
		zapCfg = zap.NewDevelopmentConfig()
	}

	level := zapcore.InfoLevel
	if cfg.Level != "" {
		if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
			return nil, fmt.Errorf("parse log level: %w", err)
		}
	}
	zapCfg.Level.SetLevel(level)

	logger, err := zapCfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	return logger, nil
}
