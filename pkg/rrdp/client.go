package rrdp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents an RRDP client
type Client struct {
	httpClient *http.Client
	maxRetries int
}

// ClientOptions configures the RRDP client
type ClientOptions struct {
	Timeout    time.Duration
	MaxRetries int
}

// NewClient creates a new RRDP client
func NewClient() *Client {
	return NewClientWithOptions(&ClientOptions{
		Timeout:    15 * time.Minute, // Much better for large snapshots
		MaxRetries: 3,
	})
}

// NewClientWithOptions creates a new RRDP client with custom options
func NewClientWithOptions(opts *ClientOptions) *Client {
	if opts == nil {
		opts = &ClientOptions{
			Timeout:    15 * time.Minute,
			MaxRetries: 3,
		}
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: opts.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
		maxRetries: opts.MaxRetries,
	}
}

// Notification represents an RRDP notification file
type Notification struct {
	XMLName   xml.Name    `xml:"notification"`
	Version   string      `xml:"version,attr"`
	SessionID string      `xml:"session_id,attr"`
	Serial    int         `xml:"serial,attr"`
	Snapshot  SnapshotRef `xml:"snapshot"`
	DeltaRefs []DeltaRef  `xml:"delta"`
}

// SnapshotRef represents a reference to a snapshot
type SnapshotRef struct {
	URI  string `xml:"uri,attr"`
	Hash string `xml:"hash,attr"`
}

// DeltaRef represents a reference to a delta
type DeltaRef struct {
	Serial int    `xml:"serial,attr"`
	URI    string `xml:"uri,attr"`
	Hash   string `xml:"hash,attr"`
}

// Snapshot represents an RRDP snapshot
type Snapshot struct {
	XMLName   xml.Name  `xml:"snapshot"`
	Version   string    `xml:"version,attr"`
	SessionID string    `xml:"session_id,attr"`
	Serial    int       `xml:"serial,attr"`
	Publishes []Publish `xml:"publish"`
}

// Delta represents an RRDP delta
type Delta struct {
	XMLName   xml.Name   `xml:"delta"`
	Version   string     `xml:"version,attr"`
	SessionID string     `xml:"session_id,attr"`
	Serial    int        `xml:"serial,attr"`
	Publishes []Publish  `xml:"publish"`
	Withdraws []Withdraw `xml:"withdraw"`
}

// Publish represents a published object
type Publish struct {
	URI  string `xml:"uri,attr"`
	Hash string `xml:"hash,attr,omitempty"`
	Data string `xml:",chardata"`
}

// Withdraw represents a withdrawn object
type Withdraw struct {
	URI  string `xml:"uri,attr"`
	Hash string `xml:"hash,attr"`
}

// FetchNotification fetches an RRDP notification file
func (c *Client) FetchNotification(url string) (*Notification, error) {
	return c.FetchNotificationContext(context.Background(), url)
}

// FetchNotificationContext fetches an RRDP notification file with context
func (c *Client) FetchNotificationContext(ctx context.Context, url string) (*Notification, error) {
	fmt.Printf("→ Fetching RRDP notification from %s\n", url)

	body, err := c.fetchWithRetry(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch notification: %w", err)
	}

	var notification Notification
	if err := xml.Unmarshal(body, &notification); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notification: %w", err)
	}

	fmt.Printf("  ✓ Got notification (serial: %d, session: %s)\n",
		notification.Serial, notification.SessionID)

	return &notification, nil
}

// FetchSnapshot fetches an RRDP snapshot
func (c *Client) FetchSnapshot(url string) (*Snapshot, error) {
	return c.FetchSnapshotWithHash(context.Background(), url, "")
}

// FetchSnapshotWithHash fetches an RRDP snapshot and verifies hash
func (c *Client) FetchSnapshotWithHash(ctx context.Context, url, expectedHash string) (*Snapshot, error) {
	fmt.Printf("→ Downloading RRDP snapshot from %s\n", url)
	fmt.Printf("  (This may take several minutes for large snapshots...)\n")

	body, err := c.fetchWithRetry(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch snapshot: %w", err)
	}

	// Verify hash if provided
	if expectedHash != "" {
		hash := sha256.Sum256(body)
		actualHash := hex.EncodeToString(hash[:])
		if actualHash != expectedHash {
			return nil, fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
		}
		fmt.Printf("  ✓ Hash verified\n")
	}

	var snapshot Snapshot
	if err := xml.Unmarshal(body, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	fmt.Printf("  ✓ Downloaded snapshot with %d objects\n", len(snapshot.Publishes))
	return &snapshot, nil
}

// fetchWithRetry fetches a URL with retry logic
func (c *Client) fetchWithRetry(ctx context.Context, url string) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * time.Second
			fmt.Printf("  Retry %d/%d after %v...\n", attempt, c.maxRetries, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		body, err := c.doFetch(ctx, url)
		if err == nil {
			return body, nil
		}

		lastErr = err
		fmt.Printf("  Attempt %d failed: %v\n", attempt+1, err)
	}

	return nil, fmt.Errorf("failed after %d retries: %w", c.maxRetries, lastErr)
}

// doFetch performs a single HTTP request with context
func (c *Client) doFetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// FetchDelta fetches an RRDP delta
func (c *Client) FetchDelta(url string) (*Delta, error) {
	return c.FetchDeltaContext(context.Background(), url)
}

// FetchDeltaContext fetches an RRDP delta with context
func (c *Client) FetchDeltaContext(ctx context.Context, url string) (*Delta, error) {
	fmt.Printf("→ Fetching RRDP delta from %s\n", url)

	body, err := c.fetchWithRetry(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch delta: %w", err)
	}

	var delta Delta
	if err := xml.Unmarshal(body, &delta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal delta: %w", err)
	}

	fmt.Printf("  ✓ Downloaded delta (serial: %d)\n", delta.Serial)
	return &delta, nil
}
