// Package analyzer orchestrates scanning, persistence, duplicate detection, and recommendations.
package analyzer

import (
	"context"
	"fmt"
	"path/filepath"

	"digital-exhaust-cleaner/internal/classifier"
	"digital-exhaust-cleaner/internal/config"
	"digital-exhaust-cleaner/internal/dedupe"
	"digital-exhaust-cleaner/internal/intelligence"
	"digital-exhaust-cleaner/internal/metadata"
	"digital-exhaust-cleaner/internal/recommendation"
	"digital-exhaust-cleaner/internal/scanner"
	"digital-exhaust-cleaner/internal/similarity"
	"digital-exhaust-cleaner/internal/storage"
	"go.uber.org/zap"
)

// Analyzer coordinates one complete local analysis run.
type Analyzer struct {
	cfg    config.Config
	logger *zap.Logger
}

// Result is the user-facing summary of an analysis run.
type Result struct {
	Root            string
	FilesScanned    int64
	DuplicateGroups []dedupe.Group
	SimilarGroups   []similarity.Group
	Classifications []classifier.Classification
	Findings        []intelligence.Finding
	Recommendations []recommendation.Recommendation
	ScanErrors      []error
}

// New creates an Analyzer.
func New(cfg config.Config, logger *zap.Logger) Analyzer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return Analyzer{cfg: cfg, logger: logger}
}

// Analyze scans root, stores metadata, detects duplicates, and ranks cleanup candidates.
func (a Analyzer) Analyze(ctx context.Context, root string) (Result, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, fmt.Errorf("resolve root path: %w", err)
	}

	store, err := storage.Open(ctx, a.cfg.App.DatabasePath)
	if err != nil {
		return Result{}, err
	}
	defer store.Close()

	session, err := store.StartScan(ctx, absoluteRoot)
	if err != nil {
		return Result{}, err
	}

	scan := scanner.New(a.cfg.Scanner, metadata.Extractor{IncludeHashes: false})
	scanResult, err := scan.Scan(ctx, absoluteRoot)
	if err != nil {
		return Result{}, err
	}

	files := dedupe.HashCandidates(ctx, scanResult.Files, a.cfg.Scanner.Workers)
	duplicateGroups := dedupe.FindExact(files)
	similarGroups := a.findSimilar(ctx, files)
	classifications := a.classify(files)
	findings := a.detectBehavior(files)
	recommendations := recommendation.New(a.cfg.Recommendation).GenerateFrom(recommendation.Input{
		Files:           files,
		DuplicateGroups: duplicateGroups,
		SimilarGroups:   similarGroups,
		Classifications: classifications,
		Findings:        findings,
	})

	if err := store.SaveFiles(ctx, session.ID, files); err != nil {
		return Result{}, err
	}
	if err := store.CompleteScan(ctx, session.ID); err != nil {
		return Result{}, err
	}

	if len(scanResult.Errors) > 0 {
		a.logger.Warn("scan completed with file-level errors", zap.Int("errors", len(scanResult.Errors)))
	}

	return Result{
		Root:            absoluteRoot,
		FilesScanned:    int64(len(files)),
		DuplicateGroups: duplicateGroups,
		SimilarGroups:   similarGroups,
		Classifications: classifications,
		Findings:        findings,
		Recommendations: recommendations,
		ScanErrors:      scanResult.Errors,
	}, nil
}

func (a Analyzer) findSimilar(ctx context.Context, files []metadata.File) []similarity.Group {
	if !a.cfg.Similarity.Enabled {
		return nil
	}
	return similarity.New(a.cfg.Similarity.HammingThreshold).FindGroups(ctx, files)
}

func (a Analyzer) classify(files []metadata.File) []classifier.Classification {
	if !a.cfg.Classifier.Enabled {
		return nil
	}
	return classifier.New().Classify(files)
}

func (a Analyzer) detectBehavior(files []metadata.File) []intelligence.Finding {
	if !a.cfg.Intelligence.Enabled {
		return nil
	}
	return intelligence.New(a.cfg.Intelligence).Analyze(files)
}
