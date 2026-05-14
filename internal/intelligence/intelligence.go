// Package intelligence detects behavioral clutter patterns from local metadata.
package intelligence

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"digital-exhaust-cleaner/internal/config"
	"digital-exhaust-cleaner/internal/metadata"
)

const (
	// PatternRepeatedDownload identifies names like "file (1).pdf".
	PatternRepeatedDownload = "repeated_download"
	// PatternAbandonedProject identifies stale project directories.
	PatternAbandonedProject = "abandoned_project"
)

var repeatedDownloadPattern = regexp.MustCompile(`(?i)\s*\(\d+\)(\.[^.]+)$`)

// Finding describes a behavioral clutter signal.
type Finding struct {
	Pattern     string
	Path        string
	Count       int
	Confidence  float64
	Explanation string
	Rules       []string
}

// Engine runs behavioral heuristics.
type Engine struct {
	cfg config.IntelligenceConfig
	now time.Time
}

// New creates an intelligence engine.
func New(cfg config.IntelligenceConfig) Engine {
	return Engine{cfg: cfg, now: time.Now().UTC()}
}

// WithNow returns a deterministic copy for tests.
func (e Engine) WithNow(now time.Time) Engine {
	e.now = now
	return e
}

// Analyze extracts behavioral findings from scanned metadata.
func (e Engine) Analyze(files []metadata.File) []Finding {
	var findings []Finding
	findings = append(findings, e.repeatedDownloads(files)...)
	findings = append(findings, e.abandonedProjects(files)...)
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Confidence > findings[j].Confidence
	})
	return findings
}

func (e Engine) repeatedDownloads(files []metadata.File) []Finding {
	byBase := make(map[string][]metadata.File)
	for _, file := range files {
		if !repeatedDownloadPattern.MatchString(file.Name) {
			continue
		}
		normalized := repeatedDownloadPattern.ReplaceAllString(file.Name, "$1")
		key := filepath.Join(strings.ToLower(file.DirectoryPath), strings.ToLower(normalized))
		byBase[key] = append(byBase[key], file)
	}

	var findings []Finding
	for key, group := range byBase {
		if len(group) < e.cfg.RepeatedDownloadMin {
			continue
		}
		findings = append(findings, Finding{
			Pattern:     PatternRepeatedDownload,
			Path:        key,
			Count:       len(group),
			Confidence:  0.82,
			Explanation: "Repeated download pattern detected from numbered filename variants.",
			Rules:       []string{"numbered_filename_variant"},
		})
	}
	return findings
}

func (e Engine) abandonedProjects(files []metadata.File) []Finding {
	type project struct {
		path       string
		count      int
		newestSeen time.Time
	}

	projects := make(map[string]project)
	for _, file := range files {
		root, ok := projectRoot(file.Path)
		if !ok {
			continue
		}
		item := projects[root]
		item.path = root
		item.count++
		if file.ModifiedAt.After(item.newestSeen) {
			item.newestSeen = file.ModifiedAt
		}
		projects[root] = item
	}

	threshold := e.now.AddDate(0, 0, -e.cfg.AbandonedProjectDays)
	var findings []Finding
	for _, project := range projects {
		if project.count < 3 || project.newestSeen.After(threshold) {
			continue
		}
		findings = append(findings, Finding{
			Pattern:     PatternAbandonedProject,
			Path:        project.path,
			Count:       project.count,
			Confidence:  0.76,
			Explanation: "Project-like directory has multiple source files and no recent modifications.",
			Rules:       []string{"project_marker", "modified_before_threshold"},
		})
	}
	return findings
}

func projectRoot(path string) (string, bool) {
	clean := filepath.Clean(path)
	parts := strings.Split(clean, string(filepath.Separator))
	for i, part := range parts {
		switch strings.ToLower(part) {
		case ".git", "go.mod", "package.json", "pyproject.toml", "Cargo.toml":
			if i == 0 {
				return filepath.Dir(clean), true
			}
			return filepath.Join(parts[:i]...), true
		}
	}
	return "", false
}
