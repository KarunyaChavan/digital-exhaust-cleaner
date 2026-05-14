// Package config loads and validates application configuration.
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config contains all runtime settings for the cleanup engine.
type Config struct {
	App            AppConfig            `yaml:"app"`
	Scanner        ScannerConfig        `yaml:"scanner"`
	Similarity     SimilarityConfig     `yaml:"similarity"`
	Classifier     ClassifierConfig     `yaml:"classifier"`
	Intelligence   IntelligenceConfig   `yaml:"intelligence"`
	Recommendation RecommendationConfig `yaml:"recommendation"`
	Logging        LoggingConfig        `yaml:"logging"`
}

// AppConfig defines local application paths.
type AppConfig struct {
	DataDir       string `yaml:"data_dir"`
	DatabasePath  string `yaml:"database_path"`
	QuarantineDir string `yaml:"quarantine_dir"`
}

// ScannerConfig controls filesystem traversal and metadata workers.
type ScannerConfig struct {
	Workers        int      `yaml:"workers"`
	FollowSymlinks bool     `yaml:"follow_symlinks"`
	IncludeHidden  bool     `yaml:"include_hidden"`
	Exclude        []string `yaml:"exclude"`
}

// SimilarityConfig controls near-duplicate media grouping.
type SimilarityConfig struct {
	Enabled          bool `yaml:"enabled"`
	HammingThreshold int  `yaml:"hamming_threshold"`
}

// ClassifierConfig controls local semantic file classification.
type ClassifierConfig struct {
	Enabled bool `yaml:"enabled"`
}

// IntelligenceConfig controls behavioral filesystem heuristics.
type IntelligenceConfig struct {
	Enabled              bool `yaml:"enabled"`
	RepeatedDownloadMin  int  `yaml:"repeated_download_min"`
	AbandonedProjectDays int  `yaml:"abandoned_project_days"`
}

// RecommendationConfig tunes cleanup recommendation scoring.
type RecommendationConfig struct {
	StaleAfterDays int     `yaml:"stale_after_days"`
	MinConfidence  float64 `yaml:"min_confidence"`
}

// LoggingConfig controls Zap logger creation.
type LoggingConfig struct {
	Level       string `yaml:"level"`
	Development bool   `yaml:"development"`
}

// Load reads a YAML config file and returns a validated Config.
func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Default returns conservative local-first application settings.
func Default() Config {
	return Config{
		App: AppConfig{
			DataDir:       ".digital-exhaust-cleaner",
			DatabasePath:  ".digital-exhaust-cleaner/cleaner.db",
			QuarantineDir: ".digital-exhaust-cleaner/quarantine",
		},
		Scanner: ScannerConfig{
			Workers:        4,
			FollowSymlinks: false,
			IncludeHidden:  false,
			Exclude:        []string{".git", "node_modules", "vendor"},
		},
		Similarity: SimilarityConfig{
			Enabled:          true,
			HammingThreshold: 8,
		},
		Classifier: ClassifierConfig{
			Enabled: true,
		},
		Intelligence: IntelligenceConfig{
			Enabled:              true,
			RepeatedDownloadMin:  3,
			AbandonedProjectDays: 90,
		},
		Recommendation: RecommendationConfig{
			StaleAfterDays: 180,
			MinConfidence:  0.55,
		},
		Logging: LoggingConfig{
			Level:       "info",
			Development: true,
		},
	}
}

// Validate rejects unsafe or incomplete configuration values.
func (c Config) Validate() error {
	if c.App.DataDir == "" {
		return errors.New("app.data_dir is required")
	}
	if c.App.DatabasePath == "" {
		return errors.New("app.database_path is required")
	}
	if c.App.QuarantineDir == "" {
		return errors.New("app.quarantine_dir is required")
	}
	if c.Scanner.Workers < 1 {
		return errors.New("scanner.workers must be greater than zero")
	}
	if c.Similarity.HammingThreshold < 0 || c.Similarity.HammingThreshold > 64 {
		return errors.New("similarity.hamming_threshold must be between 0 and 64")
	}
	if c.Intelligence.RepeatedDownloadMin < 1 {
		return errors.New("intelligence.repeated_download_min must be greater than zero")
	}
	if c.Intelligence.AbandonedProjectDays < 1 {
		return errors.New("intelligence.abandoned_project_days must be greater than zero")
	}
	if c.Recommendation.StaleAfterDays < 1 {
		return errors.New("recommendation.stale_after_days must be greater than zero")
	}
	if c.Recommendation.MinConfidence < 0 || c.Recommendation.MinConfidence > 1 {
		return errors.New("recommendation.min_confidence must be between 0 and 1")
	}
	return nil
}
