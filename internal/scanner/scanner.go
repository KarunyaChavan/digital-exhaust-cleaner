// Package scanner traverses local filesystems with explicit filtering and bounded concurrency.
package scanner

import (
	"context"
	"fmt"
	"sync"

	"digital-exhaust-cleaner/internal/config"
	"digital-exhaust-cleaner/internal/metadata"
)

// Scanner coordinates walking and metadata extraction.
type Scanner struct {
	cfg       config.ScannerConfig
	filters   Filters
	extractor metadata.Extractor
}

// Result contains metadata and non-fatal scan errors.
type Result struct {
	Files  []metadata.File
	Errors []error
}

// New creates a scanner from configuration.
func New(cfg config.ScannerConfig, extractor metadata.Extractor) Scanner {
	return Scanner{
		cfg:       cfg,
		filters:   NewFilters(cfg),
		extractor: extractor,
	}
}

// Scan traverses root and extracts metadata through a worker pool.
func (s Scanner) Scan(ctx context.Context, root string) (Result, error) {
	if s.cfg.Workers < 1 {
		return Result{}, fmt.Errorf("scanner workers must be positive")
	}

	paths := make(chan string, s.cfg.Workers*4)
	results := make(chan fileResult, s.cfg.Workers*4)

	var workers sync.WaitGroup
	for id := 0; id < s.cfg.Workers; id++ {
		workers.Add(1)
		go worker(ctx, &workers, paths, results, s.extractor)
	}

	var walkErr error
	var walkDone sync.WaitGroup
	walkDone.Add(1)
	go func() {
		defer walkDone.Done()
		walkErr = Walk(ctx, root, s.filters, paths)
		close(paths)
	}()

	go func() {
		workers.Wait()
		close(results)
	}()

	var scanResult Result
	for result := range results {
		if result.Err != nil {
			scanResult.Errors = append(scanResult.Errors, result.Err)
			continue
		}
		scanResult.Files = append(scanResult.Files, result.File)
	}

	walkDone.Wait()
	if walkErr != nil {
		return scanResult, walkErr
	}
	return scanResult, nil
}
