// Package cleanup provides reversible local cleanup operations.
package cleanup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manager quarantines files instead of deleting them.
type Manager struct {
	quarantineDir string
	historyPath   string
}

// Record describes a reversible quarantine action.
type Record struct {
	OriginalPath   string
	QuarantinePath string
	QuarantinedAt  time.Time
}

// NewManager returns a cleanup manager rooted at quarantineDir.
func NewManager(quarantineDir string) Manager {
	return Manager{
		quarantineDir: quarantineDir,
		historyPath:   filepath.Join(quarantineDir, "history.json"),
	}
}

// Quarantine moves a file into a deterministic quarantine location.
func (m Manager) Quarantine(path string) (Record, error) {
	if err := os.MkdirAll(m.quarantineDir, 0o755); err != nil {
		return Record{}, fmt.Errorf("create quarantine directory: %w", err)
	}

	target := filepath.Join(m.quarantineDir, quarantineName(path))
	if _, err := os.Stat(target); err == nil {
		target = filepath.Join(m.quarantineDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), quarantineName(path)))
	} else if !os.IsNotExist(err) {
		return Record{}, fmt.Errorf("inspect quarantine target: %w", err)
	}
	if err := os.Rename(path, target); err != nil {
		return Record{}, fmt.Errorf("quarantine file: %w", err)
	}

	record := Record{
		OriginalPath:   path,
		QuarantinePath: target,
		QuarantinedAt:  time.Now().UTC(),
	}
	if err := m.appendHistory(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Restore moves a quarantined file back to its original location.
func (m Manager) Restore(record Record) error {
	if err := os.MkdirAll(filepath.Dir(record.OriginalPath), 0o755); err != nil {
		return fmt.Errorf("create restore directory: %w", err)
	}
	if err := os.Rename(record.QuarantinePath, record.OriginalPath); err != nil {
		return fmt.Errorf("restore file: %w", err)
	}
	return nil
}

// History returns known quarantine records.
func (m Manager) History() ([]Record, error) {
	data, err := os.ReadFile(m.historyPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cleanup history: %w", err)
	}

	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse cleanup history: %w", err)
	}
	return records, nil
}

func (m Manager) appendHistory(record Record) error {
	records, err := m.History()
	if err != nil {
		return err
	}
	records = append(records, record)

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cleanup history: %w", err)
	}
	if err := os.WriteFile(m.historyPath, data, 0o600); err != nil {
		return fmt.Errorf("write cleanup history: %w", err)
	}
	return nil
}

func quarantineName(path string) string {
	hash := sha256.Sum256([]byte(path))
	base := strings.TrimSpace(filepath.Base(path))
	if base == "" {
		base = "file"
	}
	return hex.EncodeToString(hash[:8]) + "-" + base
}
