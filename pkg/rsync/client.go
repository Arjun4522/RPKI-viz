package rsync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Client represents an rsync client
type Client struct {
	// Configuration options can be added here
}

// NewClient creates a new rsync client
func NewClient() (*Client, error) {
	// Verify rsync is available
	if err := exec.Command("rsync", "--version").Run(); err != nil {
		return nil, fmt.Errorf("rsync not found in PATH (install with: apt-get install rsync)")
	}
	return &Client{}, nil
}

// Sync synchronizes an rsync URI to a local directory
func (c *Client) Sync(uri, localPath string) error {
	return c.SyncWithTimeout(context.Background(), uri, localPath, 30*time.Minute)
}

// SyncWithTimeout synchronizes with a timeout
func (c *Client) SyncWithTimeout(ctx context.Context, uri, localPath string, timeout time.Duration) error {
	// Validate path safety
	if err := validatePath(localPath); err != nil {
		return err
	}

	// Ensure the local path exists
	if err := os.MkdirAll(localPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	fmt.Printf("→ Syncing %s to %s...\n", uri, localPath)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rsync",
		"-avz",
		"--delete",
		"--timeout=60",
		"--progress",
		"--stats",
		"--human-readable",
		uri,
		localPath,
	)

	// Stream output in real-time
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("rsync timed out after %v", timeout)
		}
		return fmt.Errorf("rsync failed: %w", err)
	}

	fmt.Println("✓ Sync completed")
	return nil
}

// validatePath ensures the path is safe to use with rsync
func validatePath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// Only allow paths in /tmp/rpki-*
	if !strings.HasPrefix(absPath, "/tmp/rpki-") {
		return fmt.Errorf("path must start with /tmp/rpki-: %s", absPath)
	}

	return nil
}

// List lists files in an rsync directory
func (c *Client) List(uri string) ([]string, error) {
	// Execute rsync command to list files
	cmd := exec.Command("rsync", "--list-only", "--no-human-readable", uri)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rsync list failed: %w", err)
	}

	// Parse the output to extract file names
	lines := strings.Split(string(output), "\n")
	var files []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// rsync output format: permissions size date time name
		// Example: -rw-r--r--    1234 2024/01/15 10:30:00 file.roa
		parts := strings.Fields(line)
		if len(parts) < 5 {
			continue
		}

		// First character indicates type: - = file, d = directory, l = link
		if parts[0][0] != '-' {
			continue // Skip non-files
		}

		// File name is everything after the time (handles spaces)
		filename := strings.Join(parts[4:], " ")
		files = append(files, filename)
	}

	return files, nil
}
