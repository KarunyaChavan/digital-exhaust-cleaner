// Package tests verifies public behavior across the cleanup engine.
package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"digital-exhaust-cleaner/internal/analyzer"
	"digital-exhaust-cleaner/internal/classifier"
	"digital-exhaust-cleaner/internal/cleanup"
	"digital-exhaust-cleaner/internal/config"
	"digital-exhaust-cleaner/internal/dedupe"
	"digital-exhaust-cleaner/internal/intelligence"
	"digital-exhaust-cleaner/internal/metadata"
	"digital-exhaust-cleaner/internal/recommendation"
	"digital-exhaust-cleaner/internal/scanner"
	"digital-exhaust-cleaner/internal/similarity"
	"digital-exhaust-cleaner/internal/storage"
	"digital-exhaust-cleaner/internal/ui"
)

func TestConfigLoadAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("scanner:\n  workers: 2\nrecommendation:\n  stale_after_days: 30\n")
	mustWrite(t, path, data)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Scanner.Workers != 2 {
		t.Fatalf("workers = %d, want 2", cfg.Scanner.Workers)
	}

	cfg.Recommendation.MinConfidence = 2
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid confidence to fail")
	}
}

func TestMetadataScannerAndStorage(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.txt"), []byte("keep"))
	mustWrite(t, filepath.Join(root, ".hidden"), []byte("hidden"))
	excludedDir := filepath.Join(root, "node_modules")
	if err := os.MkdirAll(excludedDir, 0o755); err != nil {
		t.Fatalf("mkdir excluded: %v", err)
	}
	mustWrite(t, filepath.Join(excludedDir, "skip.txt"), []byte("skip"))

	scan := scanner.New(config.ScannerConfig{
		Workers:       2,
		IncludeHidden: false,
		Exclude:       []string{"node_modules"},
	}, metadata.Extractor{IncludeHashes: true})
	result, err := scan.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(result.Files))
	}
	if result.Files[0].SHA256 == "" {
		t.Fatal("expected hash")
	}

	store, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	session, err := store.StartScan(context.Background(), root)
	if err != nil {
		t.Fatalf("start scan: %v", err)
	}
	if err := store.SaveFiles(context.Background(), session.ID, result.Files); err != nil {
		t.Fatalf("save files: %v", err)
	}
	stored, err := store.ListFiles(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored files = %d, want 1", len(stored))
	}
}

func TestRecommendationSignals(t *testing.T) {
	accessed := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	files := []metadata.File{
		{Path: "a", Name: "a", SizeBytes: 10, SHA256: "same", AccessedAt: accessed},
		{Path: "b", Name: "b", SizeBytes: 10, SHA256: "same", AccessedAt: accessed},
		{Path: "downloads/report (1).pdf", Name: "report (1).pdf", DirectoryPath: "downloads"},
		{Path: "downloads/report (2).pdf", Name: "report (2).pdf", DirectoryPath: "downloads"},
		{Path: "Downloads/Screenshot_otp.png", Name: "Screenshot_otp.png", MIMEType: "image/png"},
	}

	duplicateGroups := dedupe.FindExact(files)
	classifications := classifier.New().Classify(files)
	findings := intelligence.New(config.IntelligenceConfig{
		Enabled:              true,
		RepeatedDownloadMin:  2,
		AbandonedProjectDays: 90,
	}).Analyze(files)
	recs := recommendation.New(config.RecommendationConfig{
		StaleAfterDays: 180,
		MinConfidence:  0.5,
	}).GenerateFrom(recommendation.Input{
		Files:           files,
		DuplicateGroups: duplicateGroups,
		Classifications: classifications,
		Findings:        findings,
	})

	if len(duplicateGroups) != 1 {
		t.Fatalf("duplicate groups = %d, want 1", len(duplicateGroups))
	}
	if len(classifications) == 0 {
		t.Fatal("expected classification")
	}
	if len(findings) == 0 {
		t.Fatal("expected intelligence finding")
	}
	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}
}

func TestCleanupAndReport(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "file.txt")
	mustWrite(t, original, []byte("content"))

	manager := cleanup.NewManager(filepath.Join(root, "quarantine"))
	record, err := manager.Quarantine(original)
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("expected original to move, stat err: %v", err)
	}
	if err := manager.Restore(record); err != nil {
		t.Fatalf("restore: %v", err)
	}
	history, err := manager.History()
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history = %d, want 1", len(history))
	}

	reportPath := filepath.Join(t.TempDir(), "report.html")
	if err := ui.WriteReport(reportPath, analyzerResultForTest(root)); err != nil {
		t.Fatalf("write report: %v", err)
	}
	if info, err := os.Stat(reportPath); err != nil || info.Size() == 0 {
		t.Fatalf("expected report, stat=%v err=%v", info, err)
	}
}

func TestSimilarityPrimitive(t *testing.T) {
	if got := similarity.HammingDistance(0b1010, 0b1111); got != 2 {
		t.Fatalf("distance = %d, want 2", got)
	}
}

func analyzerResultForTest(root string) analyzer.Result {
	return analyzer.Result{
		Root:         root,
		FilesScanned: 1,
		Recommendations: []recommendation.Recommendation{
			{
				Path:        filepath.Join(root, "file.txt"),
				Category:    recommendation.CategoryStale,
				Score:       0.75,
				Confidence:  0.75,
				Explanation: "Test recommendation.",
				Rules:       []string{"test_rule"},
			},
		},
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
