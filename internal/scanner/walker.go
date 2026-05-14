// File walker.go contains recursive filesystem traversal logic.
package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Walk recursively traverses root and sends accepted file paths to out.
func Walk(ctx context.Context, root string, filters Filters, out chan<- string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if path == root {
			return nil
		}

		decision, err := filters.Accept(path, entry)
		if err != nil {
			return err
		}
		if !decision.Accepted {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				return nil
			}
			return fmt.Errorf("stat walked file: %w", err)
		}
		if info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			select {
			case out <- path:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	})
}
