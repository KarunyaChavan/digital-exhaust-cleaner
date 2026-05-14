// File filters.go contains scanner inclusion and exclusion rules.
package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"digital-exhaust-cleaner/internal/config"
	"digital-exhaust-cleaner/internal/metadata"
)

// Filters applies deterministic scanner inclusion rules.
type Filters struct {
	followSymlinks bool
	includeHidden  bool
	excludedNames  map[string]struct{}
}

// Decision explains whether a path is accepted by scanner filters.
type Decision struct {
	Accepted bool
	Reason   string
}

// NewFilters builds filesystem filters from configuration.
func NewFilters(cfg config.ScannerConfig) Filters {
	excluded := make(map[string]struct{}, len(cfg.Exclude))
	for _, name := range cfg.Exclude {
		if name == "" {
			continue
		}
		excluded[strings.ToLower(filepath.Clean(name))] = struct{}{}
		excluded[strings.ToLower(filepath.Base(name))] = struct{}{}
	}

	return Filters{
		followSymlinks: cfg.FollowSymlinks,
		includeHidden:  cfg.IncludeHidden,
		excludedNames:  excluded,
	}
}

// Accept returns a decision for a path and directory entry.
func (f Filters) Accept(path string, entry fs.DirEntry) (Decision, error) {
	name := entry.Name()
	if _, ok := f.excludedNames[strings.ToLower(name)]; ok {
		return Decision{Accepted: false, Reason: "excluded name"}, nil
	}
	if !f.includeHidden && metadata.IsHiddenName(name) {
		return Decision{Accepted: false, Reason: "hidden path"}, nil
	}
	if entry.Type()&os.ModeSymlink != 0 && !f.followSymlinks {
		return Decision{Accepted: false, Reason: "symlink skipped"}, nil
	}
	return Decision{Accepted: true, Reason: "accepted"}, nil
}
