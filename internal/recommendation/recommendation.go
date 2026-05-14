// Package recommendation creates explainable cleanup recommendations.
package recommendation

import (
	"fmt"
	"sort"
	"time"

	"digital-exhaust-cleaner/internal/classifier"
	"digital-exhaust-cleaner/internal/config"
	"digital-exhaust-cleaner/internal/dedupe"
	"digital-exhaust-cleaner/internal/intelligence"
	"digital-exhaust-cleaner/internal/metadata"
	"digital-exhaust-cleaner/internal/similarity"
)

const (
	// CategoryDuplicate identifies exact duplicate cleanup candidates.
	CategoryDuplicate = "duplicate"
	// CategoryStale identifies files unused for a configured period.
	CategoryStale = "stale_file"
	// CategorySimilarImage identifies visually near-duplicate images.
	CategorySimilarImage = "similar_image"
	// CategoryTemporaryScreenshot identifies screenshots likely safe to review.
	CategoryTemporaryScreenshot = "temporary_screenshot"
	// CategoryRepeatedDownload identifies repeated download variants.
	CategoryRepeatedDownload = "repeated_download"
	// CategoryUnusedArchive identifies stale archive files.
	CategoryUnusedArchive = "unused_archive"
)

// Input bundles signals used by the recommendation engine.
type Input struct {
	Files           []metadata.File
	DuplicateGroups []dedupe.Group
	SimilarGroups   []similarity.Group
	Classifications []classifier.Classification
	Findings        []intelligence.Finding
}

// Recommendation is a human-auditable cleanup suggestion.
type Recommendation struct {
	Path         string
	Category     string
	Score        float64
	Confidence   float64
	Rules        []string
	Explanation  string
	LastAccessed time.Time
	SizeBytes    int64
}

// Engine ranks cleanup candidates using deterministic local heuristics.
type Engine struct {
	cfg config.RecommendationConfig
	now time.Time
}

// New creates a recommendation engine.
func New(cfg config.RecommendationConfig) Engine {
	return Engine{cfg: cfg, now: time.Now().UTC()}
}

// WithNow returns a copy with deterministic time for tests.
func (e Engine) WithNow(now time.Time) Engine {
	e.now = now
	return e
}

// Generate builds recommendations from file metadata and duplicate groups.
func (e Engine) Generate(files []metadata.File, duplicateGroups []dedupe.Group) []Recommendation {
	return e.GenerateFrom(Input{Files: files, DuplicateGroups: duplicateGroups})
}

// GenerateFrom builds recommendations from all local analysis signals.
func (e Engine) GenerateFrom(input Input) []Recommendation {
	var recommendations []Recommendation
	recommendations = append(recommendations, e.duplicates(input.DuplicateGroups)...)
	recommendations = append(recommendations, e.similarImages(input.SimilarGroups)...)
	recommendations = append(recommendations, e.classifications(input.Classifications, input.Files)...)
	recommendations = append(recommendations, e.behavior(input.Findings)...)
	recommendations = append(recommendations, e.stale(input.Files)...)

	filtered := recommendations[:0]
	for _, rec := range recommendations {
		if rec.Confidence >= e.cfg.MinConfidence {
			filtered = append(filtered, rec)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})
	return filtered
}

func (e Engine) duplicates(groups []dedupe.Group) []Recommendation {
	var recommendations []Recommendation
	for _, group := range groups {
		for i, file := range group.Files {
			if i == 0 {
				continue
			}
			confidence := 0.98
			score := confidence + clampSizeScore(file.SizeBytes)
			recommendations = append(recommendations, Recommendation{
				Path:         file.Path,
				Category:     CategoryDuplicate,
				Score:        score,
				Confidence:   confidence,
				Rules:        []string{"same_size", "same_sha256"},
				Explanation:  fmt.Sprintf("Exact duplicate of %s with identical size and SHA-256 hash.", group.Files[0].Path),
				LastAccessed: file.AccessedAt,
				SizeBytes:    file.SizeBytes,
			})
		}
	}
	return recommendations
}

func (e Engine) similarImages(groups []similarity.Group) []Recommendation {
	var recommendations []Recommendation
	for _, group := range groups {
		for i, file := range group.Files {
			if i == 0 {
				continue
			}
			confidence := 0.72
			score := confidence + clampSizeScore(file.SizeBytes)
			recommendations = append(recommendations, Recommendation{
				Path:         file.Path,
				Category:     CategorySimilarImage,
				Score:        score,
				Confidence:   confidence,
				Rules:        []string{"average_hash_hamming_threshold"},
				Explanation:  fmt.Sprintf("Image is visually similar to %s with perceptual hash distance up to %d.", group.Files[0].Path, group.DistanceMax),
				LastAccessed: file.AccessedAt,
				SizeBytes:    file.SizeBytes,
			})
		}
	}
	return recommendations
}

func (e Engine) classifications(classifications []classifier.Classification, files []metadata.File) []Recommendation {
	byPath := make(map[string]metadata.File, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}

	var recommendations []Recommendation
	for _, item := range classifications {
		file := byPath[item.Path]
		category, ok := categoryForLabel(item.Label)
		if !ok {
			continue
		}
		score := item.Confidence + clampSizeScore(file.SizeBytes)
		recommendations = append(recommendations, Recommendation{
			Path:         item.Path,
			Category:     category,
			Score:        score,
			Confidence:   item.Confidence,
			Rules:        item.Rules,
			Explanation:  item.Explanation,
			LastAccessed: file.AccessedAt,
			SizeBytes:    file.SizeBytes,
		})
	}
	return recommendations
}

func (e Engine) behavior(findings []intelligence.Finding) []Recommendation {
	var recommendations []Recommendation
	for _, finding := range findings {
		if finding.Pattern != intelligence.PatternRepeatedDownload {
			continue
		}
		recommendations = append(recommendations, Recommendation{
			Path:        finding.Path,
			Category:    CategoryRepeatedDownload,
			Score:       finding.Confidence,
			Confidence:  finding.Confidence,
			Rules:       finding.Rules,
			Explanation: fmt.Sprintf("%s Count: %d.", finding.Explanation, finding.Count),
		})
	}
	return recommendations
}

func (e Engine) stale(files []metadata.File) []Recommendation {
	var recommendations []Recommendation
	threshold := e.now.AddDate(0, 0, -e.cfg.StaleAfterDays)

	for _, file := range files {
		if file.AccessedAt.After(threshold) {
			continue
		}
		ageDays := e.now.Sub(file.AccessedAt).Hours() / 24
		confidence := minFloat(0.95, 0.45+(ageDays/365)*0.25)
		score := confidence + clampSizeScore(file.SizeBytes)
		recommendations = append(recommendations, Recommendation{
			Path:         file.Path,
			Category:     CategoryStale,
			Score:        score,
			Confidence:   confidence,
			Rules:        []string{"last_access_older_than_threshold"},
			Explanation:  fmt.Sprintf("File has not been accessed for %.0f days.", ageDays),
			LastAccessed: file.AccessedAt,
			SizeBytes:    file.SizeBytes,
		})
	}
	return recommendations
}

func categoryForLabel(label classifier.Label) (string, bool) {
	switch label {
	case classifier.LabelOTPScreenshot, classifier.LabelPaymentScreenshot, classifier.LabelQRScreenshot, classifier.LabelTemporaryScreenshot:
		return CategoryTemporaryScreenshot, true
	case classifier.LabelArchive:
		return CategoryUnusedArchive, true
	case classifier.LabelInstaller:
		return CategoryStale, true
	default:
		return "", false
	}
}

func clampSizeScore(size int64) float64 {
	const gib = 1024 * 1024 * 1024
	score := float64(size) / gib
	return minFloat(1, score)
}

func minFloat(a float64, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
