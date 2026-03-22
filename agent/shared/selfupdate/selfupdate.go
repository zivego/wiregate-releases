// Package selfupdate provides agent binary self-update functionality.
package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// Directive is received from the server during check-in.
type Directive struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Result describes the outcome of an update attempt.
type Result struct {
	Updated    bool
	OldVersion string
	NewVersion string
	Error      string
}

// Apply downloads the new binary and replaces the current executable.
// On success the caller should restart the process (e.g. via systemd restart).
func Apply(ctx context.Context, client *http.Client, d Directive) (Result, error) {
	if d.URL == "" || d.Version == "" {
		return Result{}, fmt.Errorf("selfupdate: empty directive")
	}

	currentBin, err := os.Executable()
	if err != nil {
		return Result{}, fmt.Errorf("selfupdate: resolve executable: %w", err)
	}
	currentBin, err = filepath.EvalSymlinks(currentBin)
	if err != nil {
		return Result{}, fmt.Errorf("selfupdate: eval symlinks: %w", err)
	}

	// Download to a temp file next to the current binary.
	dir := filepath.Dir(currentBin)
	tmpFile, err := os.CreateTemp(dir, "wiregate-agent-update-*")
	if err != nil {
		return Result{}, fmt.Errorf("selfupdate: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath) // clean up on failure
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.URL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("selfupdate: create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("selfupdate: download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("selfupdate: download failed: %s", resp.Status)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return Result{}, fmt.Errorf("selfupdate: write binary: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return Result{}, fmt.Errorf("selfupdate: close temp file: %w", err)
	}

	// Make executable.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			return Result{}, fmt.Errorf("selfupdate: chmod: %w", err)
		}
	}

	// Atomic rename: move old binary to .old, then rename new to current.
	oldPath := currentBin + ".old"
	os.Remove(oldPath) // ignore error if .old doesn't exist
	if err := os.Rename(currentBin, oldPath); err != nil {
		return Result{}, fmt.Errorf("selfupdate: backup current binary: %w", err)
	}
	if err := os.Rename(tmpPath, currentBin); err != nil {
		// Try to restore the old binary.
		_ = os.Rename(oldPath, currentBin)
		return Result{}, fmt.Errorf("selfupdate: replace binary: %w", err)
	}

	return Result{
		Updated:    true,
		NewVersion: d.Version,
	}, nil
}
