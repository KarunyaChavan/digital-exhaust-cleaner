// Package dedupe detects exact duplicate files using size and SHA-256 grouping.
package dedupe

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"digital-exhaust-cleaner/internal/metadata"
)

// Group represents files with identical content hashes.
type Group struct {
	Hash        string
	SizeBytes   int64
	Files       []metadata.File
	WastedBytes int64
}

// FindExact returns duplicate groups by size and full SHA-256 hash.
func FindExact(files []metadata.File) []Group {
	files = HashCandidates(context.Background(), files, 1)
	bySize := make(map[int64][]metadata.File)
	for _, file := range files {
		if file.SizeBytes == 0 || file.SHA256 == "" {
			continue
		}
		bySize[file.SizeBytes] = append(bySize[file.SizeBytes], file)
	}

	var groups []Group
	for size, candidates := range bySize {
		if len(candidates) < 2 {
			continue
		}

		byHash := make(map[string][]metadata.File)
		for _, file := range candidates {
			byHash[file.SHA256] = append(byHash[file.SHA256], file)
		}
		for hash, duplicateFiles := range byHash {
			if len(duplicateFiles) < 2 {
				continue
			}
			sort.Slice(duplicateFiles, func(i, j int) bool {
				return duplicateFiles[i].Path < duplicateFiles[j].Path
			})
			groups = append(groups, Group{
				Hash:        hash,
				SizeBytes:   size,
				Files:       duplicateFiles,
				WastedBytes: size * int64(len(duplicateFiles)-1),
			})
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].WastedBytes > groups[j].WastedBytes
	})
	return groups
}

// HashCandidates hashes only files that have at least one same-size peer.
func HashCandidates(ctx context.Context, files []metadata.File, workers int) []metadata.File {
	if workers < 1 {
		workers = 1
	}

	bySize := make(map[int64]int, len(files))
	for _, file := range files {
		bySize[file.SizeBytes]++
	}

	out := make([]metadata.File, len(files))
	copy(out, files)

	jobs := make(chan int, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				file := out[index]
				if file.SHA256 != "" || file.IsSymlink {
					continue
				}
				partial, full, err := metadata.HashFile(file.Path)
				if err != nil {
					out[index].HashError = fmt.Sprintf("%v", err)
					continue
				}
				out[index].PartialSHA256 = partial
				out[index].SHA256 = full
			}
		}()
	}

	for index, file := range out {
		if file.SizeBytes > 0 && bySize[file.SizeBytes] > 1 {
			select {
			case jobs <- index:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return out
			}
		}
	}
	close(jobs)
	wg.Wait()
	return out
}
