package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"digital-exhaust-cleaner/internal/config"
	"digital-exhaust-cleaner/internal/metadata"
	"digital-exhaust-cleaner/internal/scanner"
)

func BenchmarkScanSmallTree(b *testing.B) {
	root := b.TempDir()
	for i := 0; i < 1000; i++ {
		path := filepath.Join(root, fmt.Sprintf("file-%04d.txt", i))
		if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
			b.Fatalf("write file: %v", err)
		}
	}

	scan := scanner.New(config.ScannerConfig{Workers: 4}, metadata.Extractor{})
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := scan.Scan(context.Background(), root); err != nil {
			b.Fatalf("scan: %v", err)
		}
	}
}
