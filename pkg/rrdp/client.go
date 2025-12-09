package rrdp

import (
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
}

// NewClient creates a new RRDP client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
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
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Verify hash if provided
	// (In a real implementation, you would verify against the expected hash)

	var notification Notification
	if err := xml.Unmarshal(body, &notification); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notification: %w", err)
	}

	return &notification, nil
}

// FetchSnapshot fetches an RRDP snapshot
func (c *Client) FetchSnapshot(url string) (*Snapshot, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Calculate hash for verification
	hash := sha256.Sum256(body)
	_ = hex.EncodeToString(hash[:]) // TODO: Verify against expected hash in production

	var snapshot Snapshot
	if err := xml.Unmarshal(body, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	return &snapshot, nil
}

// FetchDelta fetches an RRDP delta
func (c *Client) FetchDelta(url string) (*Delta, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch delta: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var delta Delta
	if err := xml.Unmarshal(body, &delta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal delta: %w", err)
	}

	return &delta, nil
}
