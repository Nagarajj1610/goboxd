// Package sandbox manages the per-request jail directory lifecycle.
// Holes 2, 5, and 7 are closed here.
package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

const staleThreshold = 10 * time.Minute

// Sandbox holds the path of a single request's jail directory.
type Sandbox struct {
	Dir    string
	logger *zap.Logger
}

// New creates a unique jail directory inside baseDir.
// HOLE 5 FIX: os.MkdirTemp gives OS-guaranteed unique names.
// HOLE 2 FIX: os.MkdirAll used instead of shell commands.
// Security: file:internal/sandbox/sandbox.go (New)
func New(baseDir string, logger *zap.Logger) (*Sandbox, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("sandbox: cannot create base dir %s: %w", baseDir, err)
	}
	dir, err := os.MkdirTemp(baseDir, "jail-")
	if err != nil {
		return nil, fmt.Errorf("sandbox: cannot create jail dir in %s: %w", baseDir, err)
	}
	if err := os.Chmod(dir, 0o1777); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("sandbox: cannot chmod jail dir %s: %w", dir, err)
	}
	return &Sandbox{Dir: dir, logger: logger}, nil
}

// Cleanup removes the jail directory and all its contents.
// HOLE 2 FIX: os.RemoveAll used instead of shell commands.
// HOLE 7 FIX: Called via defer immediately after New.
// Security: file:internal/sandbox/sandbox.go (Cleanup)
func (s *Sandbox) Cleanup() {
	if err := os.RemoveAll(s.Dir); err != nil {
		s.logger.Warn("sandbox: failed to remove jail dir",
			zap.String("dir", s.Dir),
			zap.Error(err),
		)
	}
}

// WriteFile writes content to a file inside the jail.
// HOLE 2 FIX: filepath.Join used for all path construction.
// Security: file:internal/sandbox/sandbox.go (WriteFile)
func (s *Sandbox) WriteFile(filename, content string) (string, error) {
	fullPath := filepath.Join(s.Dir, filename)
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("sandbox: cannot write %s: %w", filename, err)
	}
	return fullPath, nil
}

// SweepStale removes jail directories older than 10 minutes from baseDir.
// HOLE 7 FIX: Called at server startup to remove orphans from crashes.
// Security: file:internal/sandbox/sandbox.go (SweepStale)
func SweepStale(baseDir string, logger *zap.Logger) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() || len(entry.Name()) < 5 || entry.Name()[:5] != "jail-" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		age := now.Sub(info.ModTime())
		if age < staleThreshold {
			continue
		}
		fullPath := filepath.Join(baseDir, entry.Name())
		if err := os.RemoveAll(fullPath); err != nil {
			logger.Warn("sandbox: failed to remove stale jail dir",
				zap.String("dir", fullPath), zap.Duration("age", age), zap.Error(err))
		} else {
			logger.Info("sandbox: removed stale jail dir",
				zap.String("dir", fullPath), zap.Duration("age", age))
		}
	}
}
