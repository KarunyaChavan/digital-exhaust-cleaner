// Package similarity detects near-duplicate images using average hashes.
package similarity

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math/bits"
	"os"
	"sort"
	"strings"

	"digital-exhaust-cleaner/internal/metadata"
)

const hashSize = 8

// Fingerprint is a compact perceptual image hash.
type Fingerprint struct {
	Path string
	Hash uint64
}

// Group contains visually similar image files.
type Group struct {
	Files       []metadata.File
	DistanceMax int
}

// Engine groups near-duplicate images.
type Engine struct {
	Threshold int
}

// New returns a similarity engine with a normalized threshold.
func New(threshold int) Engine {
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 64 {
		threshold = 64
	}
	return Engine{Threshold: threshold}
}

// FindGroups fingerprints images and groups files within the hamming threshold.
func (e Engine) FindGroups(ctx context.Context, files []metadata.File) []Group {
	fingerprints := make([]Fingerprint, 0, len(files))
	byPath := make(map[string]metadata.File, len(files))
	for _, file := range files {
		if ctx.Err() != nil {
			return nil
		}
		if !isSupportedImage(file) {
			continue
		}
		hash, err := AverageHash(file.Path)
		if err != nil {
			continue
		}
		fingerprints = append(fingerprints, Fingerprint{Path: file.Path, Hash: hash})
		byPath[file.Path] = file
	}

	visited := make([]bool, len(fingerprints))
	var groups []Group
	for i := range fingerprints {
		if visited[i] {
			continue
		}
		group := []metadata.File{byPath[fingerprints[i].Path]}
		visited[i] = true
		maxDistance := 0
		for j := i + 1; j < len(fingerprints); j++ {
			if visited[j] {
				continue
			}
			distance := HammingDistance(fingerprints[i].Hash, fingerprints[j].Hash)
			if distance <= e.Threshold {
				visited[j] = true
				group = append(group, byPath[fingerprints[j].Path])
				if distance > maxDistance {
					maxDistance = distance
				}
			}
		}
		if len(group) > 1 {
			sort.Slice(group, func(a, b int) bool {
				return group[a].Path < group[b].Path
			})
			groups = append(groups, Group{Files: group, DistanceMax: maxDistance})
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i].Files) > len(groups[j].Files)
	})
	return groups
}

// AverageHash computes an 8x8 grayscale average hash.
func AverageHash(path string) (uint64, error) {
	handle, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open image: %w", err)
	}
	defer handle.Close()

	img, _, err := image.Decode(handle)
	if err != nil {
		return 0, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		return 0, fmt.Errorf("empty image")
	}

	values := make([]uint32, 0, hashSize*hashSize)
	var total uint64
	for y := 0; y < hashSize; y++ {
		for x := 0; x < hashSize; x++ {
			srcX := bounds.Min.X + x*bounds.Dx()/hashSize
			srcY := bounds.Min.Y + y*bounds.Dy()/hashSize
			r, g, b, _ := img.At(srcX, srcY).RGBA()
			gray := uint32((299*r + 587*g + 114*b) / 1000)
			values = append(values, gray)
			total += uint64(gray)
		}
	}

	average := uint32(total / uint64(len(values)))
	var hash uint64
	for i, value := range values {
		if value >= average {
			hash |= 1 << uint(i)
		}
	}
	return hash, nil
}

// HammingDistance returns the number of differing hash bits.
func HammingDistance(a uint64, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

func isSupportedImage(file metadata.File) bool {
	if !strings.HasPrefix(file.MIMEType, "image/") {
		return false
	}
	switch strings.ToLower(file.Extension) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return true
	default:
		return false
	}
}
