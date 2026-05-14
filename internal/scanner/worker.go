// File worker.go contains scanner worker-pool execution.
package scanner

import (
	"context"
	"sync"

	"digital-exhaust-cleaner/internal/metadata"
)

type fileResult struct {
	File metadata.File
	Err  error
}

func worker(ctx context.Context, wg *sync.WaitGroup, paths <-chan string, results chan<- fileResult, extractor metadata.Extractor) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case path, ok := <-paths:
			if !ok {
				return
			}
			file, err := extractor.Extract(path)
			select {
			case results <- fileResult{File: file, Err: err}:
			case <-ctx.Done():
				return
			}
		}
	}
}
