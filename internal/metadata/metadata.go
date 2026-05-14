// Package metadata extracts deterministic file metadata without reading whole files into memory.
package metadata

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const partialHashBytes int64 = 64 * 1024

// File contains normalized metadata used by storage, dedupe, and recommendation packages.
type File struct {
	Path          string
	DirectoryPath string
	Name          string
	Extension     string
	MIMEType      string
	SizeBytes     int64
	CreatedAt     *time.Time
	ModifiedAt    time.Time
	AccessedAt    time.Time
	PathEntropy   float64
	SHA256        string
	PartialSHA256 string
	HashError     string
	IsHidden      bool
	IsSymlink     bool
}

// Extractor builds metadata records for filesystem paths.
type Extractor struct {
	IncludeHashes bool
}

// Extract returns metadata for a single file path.
func (e Extractor) Extract(path string) (File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return File{}, fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return File{}, fmt.Errorf("metadata extraction expects a file: %s", path)
	}

	file := File{
		Path:          path,
		DirectoryPath: filepath.Dir(path),
		Name:          info.Name(),
		Extension:     strings.ToLower(filepath.Ext(path)),
		SizeBytes:     info.Size(),
		ModifiedAt:    info.ModTime().UTC(),
		AccessedAt:    accessedAt(info).UTC(),
		PathEntropy:   PathEntropy(path),
		IsHidden:      IsHiddenName(info.Name()),
		IsSymlink:     info.Mode()&os.ModeSymlink != 0,
	}
	file.CreatedAt = createdAt(info)
	file.MIMEType = detectMIME(path, file.Extension)

	if e.IncludeHashes && !file.IsSymlink {
		partial, full, err := HashFile(path)
		if err != nil {
			return File{}, err
		}
		file.PartialSHA256 = partial
		file.SHA256 = full
	}

	return file, nil
}

// HashFile calculates partial and full SHA-256 hashes using streaming reads.
func HashFile(path string) (string, string, error) {
	handle, err := os.Open(path)
	if err != nil {
		return "", "", fmt.Errorf("open file for hashing: %w", err)
	}
	defer handle.Close()

	partialHasher := sha256.New()
	fullHasher := sha256.New()
	buffer := make([]byte, 128*1024)
	var readTotal int64

	for {
		n, readErr := handle.Read(buffer)
		if n > 0 {
			chunk := buffer[:n]
			if _, err := fullHasher.Write(chunk); err != nil {
				return "", "", fmt.Errorf("hash file: %w", err)
			}
			if readTotal < partialHashBytes {
				remaining := partialHashBytes - readTotal
				if int64(n) > remaining {
					chunk = chunk[:int(remaining)]
				}
				if _, err := partialHasher.Write(chunk); err != nil {
					return "", "", fmt.Errorf("hash partial file: %w", err)
				}
			}
			readTotal += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", "", fmt.Errorf("read file for hashing: %w", readErr)
		}
	}

	return hex.EncodeToString(partialHasher.Sum(nil)), hex.EncodeToString(fullHasher.Sum(nil)), nil
}

// PathEntropy scores path-name randomness using Shannon entropy.
func PathEntropy(path string) float64 {
	if path == "" {
		return 0
	}

	counts := make(map[rune]float64)
	for _, char := range strings.ToLower(path) {
		counts[char]++
	}

	length := float64(len([]rune(path)))
	var entropy float64
	for _, count := range counts {
		probability := count / length
		entropy -= probability * math.Log2(probability)
	}
	return entropy
}

// IsHiddenName detects dot-prefixed hidden names across platforms.
func IsHiddenName(name string) bool {
	return strings.HasPrefix(name, ".") && name != "." && name != ".."
}

func detectMIME(path string, extension string) string {
	if byExtension := mime.TypeByExtension(extension); byExtension != "" {
		return byExtension
	}

	handle, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer handle.Close()

	buffer := make([]byte, 512)
	n, err := handle.Read(buffer)
	if err != nil && err != io.EOF {
		return "application/octet-stream"
	}
	return http.DetectContentType(buffer[:n])
}

func accessedAt(info os.FileInfo) time.Time {
	// Go does not expose portable access time through os.FileInfo.
	// The modified time is used as a conservative fallback.
	return info.ModTime()
}

func createdAt(info os.FileInfo) *time.Time {
	// Creation time is platform-specific and intentionally left unset unless
	// a platform adapter is added in a later phase.
	return nil
}
