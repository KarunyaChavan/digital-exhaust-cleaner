// File server.go hosts the interactive local cleanup UI.
package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"digital-exhaust-cleaner/internal/analyzer"
	"digital-exhaust-cleaner/internal/cleanup"
)

const defaultServerTimeout = 15 * time.Second

// ServerConfig defines the local interactive UI server.
type ServerConfig struct {
	Addr          string
	Root          string
	QuarantineDir string
	Result        analyzer.Result
}

// Serve starts a loopback-only interactive cleanup server.
func Serve(ctx context.Context, cfg ServerConfig) error {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8787"
	}
	if err := ensureLoopback(cfg.Addr); err != nil {
		return err
	}

	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return fmt.Errorf("resolve server root: %w", err)
	}

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: defaultServerTimeout,
	}
	manager := cleanup.NewManager(cfg.QuarantineDir)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := reportTemplate.Execute(w, viewModel{Result: cfg.Result, Interactive: true}); err != nil {
			http.Error(w, "render report", http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/api/quarantine", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var request struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		target, err := filepath.Abs(request.Path)
		if err != nil || !isInside(root, target) {
			http.Error(w, "path is outside the scanned root", http.StatusBadRequest)
			return
		}

		record, err := manager.Quarantine(target)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(record); err != nil {
			http.Error(w, "encode response", http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		history, err := manager.History()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(history); err != nil {
			http.Error(w, "encode response", http.StatusInternalServerError)
		}
	})

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultServerTimeout)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func ensureLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse server address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("interactive UI must bind to a loopback address")
	}
	return nil
}

func isInside(root string, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
