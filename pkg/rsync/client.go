package rsync

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client represents an rsync client
type Client struct {
	// Configuration options can be added here
}

// NewClient creates a new rsync client
func NewClient() *Client {
	return &Client{}
}

// Sync synchronizes an rsync URI to a local directory
func (c *Client) Sync(uri, localPath string) error {
	// Ensure the local path exists
	if err := c.ensurePath(localPath); err != nil {
		return fmt.Errorf("failed to ensure local path: %w", err)
	}

	// Execute rsync command
	// Note: This assumes rsync is installed on the system
	cmd := exec.Command("rsync", "-avz", "--delete", uri, localPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync failed: %w, output: %s", err, string(output))
	}

	return nil
}

// List lists files in an rsync directory
func (c *Client) List(uri string) ([]string, error) {
	// Execute rsync command to list files
	cmd := exec.Command("rsync", "--list-only", uri)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rsync list failed: %w", err)
	}

	// Parse the output to extract file names
	lines := strings.Split(string(output), "\n")
	var files []string

	for _, line := range lines {
		// Skip empty lines
		if line == "" {
			continue
		}

		// Extract file name (this is a simplified parsing)
		parts := strings.Fields(line)
		if len(parts) >= 5 {
			// The file name is typically the last part
			filename := parts[len(parts)-1]
			files = append(files, filename)
		}
	}

	return files, nil
}

// ensurePath ensures that the given path exists
func (c *Client) ensurePath(path string) error {
	return exec.Command("mkdir", "-p", filepath.Dir(path)).Run()
}
