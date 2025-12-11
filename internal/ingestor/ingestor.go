package ingestor

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/rpki-viz/backend/internal/model"
	"github.com/rpki-viz/backend/internal/service"
	"github.com/rpki-viz/backend/pkg/rrdp"
	"github.com/rpki-viz/backend/pkg/rsync"
)

// IngestionCache provides in-memory caching for database lookups
type IngestionCache struct {
	asns     sync.Map // map[int]*model.ASN
	prefixes sync.Map // map[string]*model.Prefix
	ttl      time.Duration
}

// WorkerPool manages concurrent task execution with limited workers
type WorkerPool struct {
	workers   int
	taskQueue chan Task
	wg        sync.WaitGroup
	ingestor  *Ingestor
}

// Task represents a unit of work for the worker pool
type Task struct {
	Name string
	Fn   func(context.Context) error
}

// NewWorkerPool creates a new worker pool with specified number of workers
func NewWorkerPool(workers int, ingestor *Ingestor) *WorkerPool {
	return &WorkerPool{
		workers:   workers,
		taskQueue: make(chan Task, workers*2), // Buffer for smooth operation
		ingestor:  ingestor,
	}
}

// Start begins processing tasks
func (wp *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(ctx, wp.ingestor)
	}
}

// Execute submits tasks and waits for completion
func (wp *WorkerPool) Execute(ctx context.Context, tasks []Task) error {
	wp.Start(ctx)

	// Send tasks to workers
	go func() {
		defer close(wp.taskQueue)
		for _, task := range tasks {
			select {
			case wp.taskQueue <- task:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for all workers to complete
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// worker processes tasks from the queue
func (wp *WorkerPool) worker(ctx context.Context, ingestor *Ingestor) {
	defer wp.wg.Done()
	for {
		select {
		case task, ok := <-wp.taskQueue:
			if !ok {
				return // Channel closed, no more tasks
			}
			if err := ingestor.ingestWithRetry(ctx, task.Name, task.Fn); err != nil {
				fmt.Printf("Final error ingesting from %s: %v\n", task.Name, err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// IngestionConfig holds configuration for ingestion operations
type IngestionConfig struct {
	MaxRetries      int
	RetryBackoff    time.Duration
	Timeout         time.Duration
	SkipOnFailure   bool
	ValidationLevel ValidationLevel
}

// ValidationLevel represents the strictness of validation
type ValidationLevel int

const (
	Strict ValidationLevel = iota
	Lenient
	VeryLenient
)

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	StateClosed CircuitBreakerState = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern for TA ingestion
type CircuitBreaker struct {
	mu               sync.RWMutex
	failureThreshold int
	timeout          time.Duration
	state            CircuitBreakerState
	lastFailureTime  time.Time
	failureCount     int
	successCount     int
	nextAttemptTime  time.Time
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		timeout:          timeout,
		state:            StateClosed,
	}
}

// CanExecute checks if the circuit breaker allows execution
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Now().After(cb.nextAttemptTime) {
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = StateHalfOpen
			cb.successCount = 0
			cb.mu.Unlock()
			cb.mu.RLock()
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful execution
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.successCount++

	// If in half-open state and we've had enough successes, close the circuit
	if cb.state == StateHalfOpen && cb.successCount >= 1 {
		cb.state = StateClosed
	}
}

// RecordFailure records a failed execution
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.failureCount >= cb.failureThreshold {
		cb.state = StateOpen
		cb.nextAttemptTime = time.Now().Add(cb.timeout)
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Ingestor handles data ingestion from various RPKI sources
type Ingestor struct {
	rrdpClient      *rrdp.Client
	rsyncClient     *rsync.Client
	roaProcessor    *service.ROAProcessor
	httpClient      *http.Client
	cache           *IngestionCache
	config          *IngestionConfig
	circuitBreakers sync.Map // map[string]*CircuitBreaker
	dbClient        interface {
		GetOrCreateASN(ctx context.Context, number int, name, country string) (*model.ASN, error)
		GetASNByID(ctx context.Context, id string) (*model.ASN, error)
		GetOrCreateTrustAnchor(ctx context.Context, name, uri, rsaKey, sha256 string) (*model.TrustAnchor, error)
		GetPrefixByCIDR(ctx context.Context, cidr string) (*model.Prefix, error)
		UpsertPrefix(ctx context.Context, prefix *model.Prefix) (*model.Prefix, error)
		InsertPrefix(ctx context.Context, prefix *model.Prefix) error
		InsertROA(ctx context.Context, roa *model.ROA) error
		BatchInsertVRPs(ctx context.Context, vrps []*model.VRP) error
		InsertVRP(ctx context.Context, vrp *model.VRP) error
	}
}

// NewIngestor creates a new ingestor
func NewIngestor(dbClient interface {
	GetOrCreateASN(ctx context.Context, number int, name, country string) (*model.ASN, error)
	GetASNByID(ctx context.Context, id string) (*model.ASN, error)
	GetOrCreateTrustAnchor(ctx context.Context, name, uri, rsaKey, sha256 string) (*model.TrustAnchor, error)
	GetPrefixByCIDR(ctx context.Context, cidr string) (*model.Prefix, error)
	UpsertPrefix(ctx context.Context, prefix *model.Prefix) (*model.Prefix, error)
	InsertPrefix(ctx context.Context, prefix *model.Prefix) error
	InsertROA(ctx context.Context, roa *model.ROA) error
	BatchInsertVRPs(ctx context.Context, vrps []*model.VRP) error
	InsertVRP(ctx context.Context, vrp *model.VRP) error
}) (*Ingestor, error) {
	rsyncClient, err := rsync.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create rsync client: %w", err)
	}

	roaProc := service.NewROAProcessor()
	roaProc.SetDBClient(dbClient)

	return &Ingestor{
		rrdpClient:   rrdp.NewClient(),
		rsyncClient:  rsyncClient,
		roaProcessor: roaProc,
		httpClient: &http.Client{
			Timeout: 15 * time.Minute, // Increased for large RRDP snapshots
			Transport: &http.Transport{
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
				DisableKeepAlives:   false,
			},
		},
		cache: &IngestionCache{
			ttl: 10 * time.Minute, // Cache for 10 minutes during ingestion
		},
		config: &IngestionConfig{
			MaxRetries:      3,
			RetryBackoff:    5 * time.Second,
			Timeout:         15 * time.Minute, // Match HTTP client timeout
			SkipOnFailure:   false,
			ValidationLevel: Lenient,
		},
		dbClient: dbClient,
	}, nil
}

// ensureTrustAnchors creates default trust anchors if they don't exist
func (i *Ingestor) ensureTrustAnchors(ctx context.Context) error {
	trustAnchors := []struct {
		name   string
		uri    string
		rsaKey string
		sha256 string
	}{
		{
			name:   "RIPE NCC",
			uri:    "rsync://rpki.ripe.net/repository/",
			rsaKey: "",
			sha256: "",
		},
		{
			name:   "AFRINIC",
			uri:    "rsync://rpki.afrinic.net/repository/",
			rsaKey: "",
			sha256: "",
		},
		{
			name:   "APNIC",
			uri:    "rsync://rpki.apnic.net/repository/",
			rsaKey: "",
			sha256: "",
		},
		{
			name:   "ARIN",
			uri:    "rsync://rpki.arin.net/repository/",
			rsaKey: "",
			sha256: "",
		},
		{
			name:   "LACNIC",
			uri:    "rsync://repository.lacnic.net/rpki/",
			rsaKey: "",
			sha256: "",
		},
		{
			name:   "Cloudflare",
			uri:    "https://rpki.cloudflare.com/",
			rsaKey: "",
			sha256: "",
		},
	}

	for _, ta := range trustAnchors {
		_, err := i.dbClient.GetOrCreateTrustAnchor(ctx, ta.name, ta.uri, ta.rsaKey, ta.sha256)
		if err != nil {
			return fmt.Errorf("failed to ensure trust anchor %s: %w", ta.name, err)
		}
	}

	return nil
}

// IngestFromRIPE ingests data from RIPE NCC sources
func (i *Ingestor) IngestFromRIPE(ctx context.Context) error {
	return i.ingestFromRRDPWithFallback(
		ctx,
		"RIPE NCC",
		"https://rrdp.ripe.net/notification.xml",
		"rsync://rpki.ripe.net/repository/",
	)
}

// IngestFromAFRINIC ingests data from AFRINIC sources
func (i *Ingestor) IngestFromAFRINIC(ctx context.Context) error {
	// AFRINIC RRDP is unreliable - use extended timeout and prefer rsync fallback
	fmt.Println("Note: AFRINIC RRDP has known performance issues, using extended timeout")

	// Try RRDP with longer timeout (20 minutes for AFRINIC)
	rrdpCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	return i.ingestFromRRDPWithFallback(
		rrdpCtx,
		"AFRINIC",
		"https://rrdp.afrinic.net/notification.xml",
		"rsync://rpki.afrinic.net/repository/",
	)
}

// IngestFromAPNIC ingests data from APNIC sources
func (i *Ingestor) IngestFromAPNIC(ctx context.Context) error {
	return i.ingestFromRRDPWithFallback(
		ctx,
		"APNIC",
		"https://rrdp.apnic.net/notification.xml",
		"rsync://rpki.apnic.net/repository/",
	)
}

// IngestFromARIN ingests data from ARIN sources
func (i *Ingestor) IngestFromARIN(ctx context.Context) error {
	return i.ingestFromRRDPWithFallback(
		ctx,
		"ARIN",
		"https://rrdp.arin.net/notification.xml",
		"rsync://rpki.arin.net/repository/",
	)
}

// IngestFromLACNIC ingests data from LACNIC sources
func (i *Ingestor) IngestFromLACNIC(ctx context.Context) error {
	return i.ingestFromRRDPWithFallback(
		ctx,
		"LACNIC",
		"https://rrdp.lacnic.net/rrdp/notification.xml",
		"rsync://repository.lacnic.net/rpki/",
	)
}

// IngestFromCloudflare ingests data from Cloudflare sources
func (i *Ingestor) IngestFromCloudflare(ctx context.Context) error {
	// Ingest from Cloudflare RPKI JSON feed
	if err := i.ingestFromCloudflareJSON(ctx); err != nil {
		return fmt.Errorf("failed to ingest from Cloudflare JSON: %w", err)
	}

	return nil
}

// ingestFromRIPEstat ingests from RIPEstat RPKI Validation API
func (i *Ingestor) ingestFromRIPEstat(ctx context.Context) error {
	url := "https://stat.ripe.net/data/rpki-validation/data.json"

	resp, err := i.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Parse RIPEstat response and extract ROAs/VRPs
	roas, vrps, err := i.processRIPEData(ctx, body)
	if err != nil {
		return fmt.Errorf("failed to process RIPE data: %w", err)
	}

	fmt.Printf("Ingested from RIPEstat: %d ROAs, %d VRPs\n", len(roas), len(vrps))
	return nil
}

// ingestFromRIPERRDP ingests from RIPE RRDP
func (i *Ingestor) ingestFromRIPERRDP(ctx context.Context) error {
	notificationURL := "https://rrdp.ripe.net/notification.xml"

	notification, err := i.rrdpClient.FetchNotification(notificationURL)
	if err != nil {
		return err
	}

	fmt.Printf("Fetched RRDP notification with serial %d\n", notification.Serial)

	// Fetch snapshot or deltas as needed
	if notification.Snapshot.URI != "" {
		snapshot, err := i.rrdpClient.FetchSnapshot(notification.Snapshot.URI)
		if err != nil {
			return err
		}

		fmt.Printf("Fetched snapshot with %d publishes\n", len(snapshot.Publishes))

		for _, publish := range snapshot.Publishes {
			if strings.HasSuffix(publish.URI, ".roa") {
				// Decode base64 data from RRDP (clean whitespace first)
				cleanData := strings.Map(func(r rune) rune {
					if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
						return r
					}
					return -1
				}, publish.Data)
				decodedData, err := base64.StdEncoding.DecodeString(cleanData)
				if err != nil {
					// Skip invalid base64 data (some RIRs may have encoding issues)
					fmt.Printf("Skipping ROA %s due to base64 decoding error: %v\n", publish.URI, err)
					continue
				}

				// Get RIPE trust anchor
				ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, "RIPE NCC", "rsync://rpki.ripe.net/repository/", "", "")
				if err != nil {
					fmt.Printf("Error getting RIPE trust anchor for ROA %s: %v\n", publish.URI, err)
					continue
				}

				vrps, err := i.roaProcessor.ExtractVRPsFromROAFile(ctx, decodedData, ta)
				if err != nil {
					fmt.Printf("Error processing ROA %s: %v\n", publish.URI, err)
					continue
				}

				fmt.Printf("Processed ROA %s: %d VRPs\n", publish.URI, len(vrps))
			}
		}
	}

	return nil
}

// ingestFromRIPERsync ingests from RIPE rsync
func (i *Ingestor) ingestFromRIPERsync(ctx context.Context) error {
	rsyncURI := "rsync://rpki.ripe.net/repository/"
	localPath := "/tmp/rpki-ripe"

	if err := i.rsyncClient.Sync(rsyncURI, localPath); err != nil {
		return err
	}

	fmt.Printf("Synced RIPE repository to %s\n", localPath)
	return nil
}

// ingestFromAFRINICRRDP ingests from AFRINIC RRDP
func (i *Ingestor) ingestFromAFRINICRRDP(ctx context.Context) error {
	notificationURL := "https://rrdp.afrinic.net/notification.xml"

	notification, err := i.rrdpClient.FetchNotification(notificationURL)
	if err != nil {
		return err
	}

	fmt.Printf("Fetched AFRINIC RRDP notification with serial %d\n", notification.Serial)

	if notification.Snapshot.URI != "" {
		snapshot, err := i.rrdpClient.FetchSnapshot(notification.Snapshot.URI)
		if err != nil {
			return err
		}

		fmt.Printf("Fetched AFRINIC snapshot with %d publishes\n", len(snapshot.Publishes))

		for _, publish := range snapshot.Publishes {
			if strings.HasSuffix(publish.URI, ".roa") {
				// Decode base64 data from RRDP (clean whitespace first)
				cleanData := strings.Map(func(r rune) rune {
					if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
						return r
					}
					return -1
				}, publish.Data)
				decodedData, err := base64.StdEncoding.DecodeString(cleanData)
				if err != nil {
					// Skip invalid base64 data (some RIRs may have encoding issues)
					fmt.Printf("Skipping ROA %s due to base64 decoding error: %v\n", publish.URI, err)
					continue
				}

				// Get AFRINIC trust anchor
				ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, "AFRINIC", "rsync://rpki.afrinic.net/repository/", "", "")
				if err != nil {
					fmt.Printf("Error getting AFRINIC trust anchor for ROA %s: %v\n", publish.URI, err)
					continue
				}

				vrps, err := i.roaProcessor.ExtractVRPsFromROAFile(ctx, decodedData, ta)
				if err != nil {
					fmt.Printf("Error processing ROA %s: %v\n", publish.URI, err)
					continue
				}

				fmt.Printf("Processed ROA %s: %d VRPs\n", publish.URI, len(vrps))
			}
		}
	}

	return nil
}

// ingestFromAFRINICRsync ingests from AFRINIC rsync
func (i *Ingestor) ingestFromAFRINICRsync(ctx context.Context) error {
	rsyncURI := "rsync://rpki.afrinic.net/repository/"
	localPath := "/tmp/rpki-afrinic"

	if err := i.rsyncClient.Sync(rsyncURI, localPath); err != nil {
		return err
	}

	fmt.Printf("Synced AFRINIC repository to %s\n", localPath)
	return nil
}

// ingestFromAPNICRRDP ingests from APNIC RRDP
func (i *Ingestor) ingestFromAPNICRRDP(ctx context.Context) error {
	notificationURL := "https://rrdp.apnic.net/notification.xml"

	notification, err := i.rrdpClient.FetchNotification(notificationURL)
	if err != nil {
		return err
	}

	fmt.Printf("Fetched APNIC RRDP notification with serial %d\n", notification.Serial)

	if notification.Snapshot.URI != "" {
		snapshot, err := i.rrdpClient.FetchSnapshot(notification.Snapshot.URI)
		if err != nil {
			return err
		}

		fmt.Printf("Fetched APNIC snapshot with %d publishes\n", len(snapshot.Publishes))

		for _, publish := range snapshot.Publishes {
			if strings.HasSuffix(publish.URI, ".roa") {
				// Decode base64 data from RRDP (clean whitespace first)
				cleanData := strings.Map(func(r rune) rune {
					if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
						return r
					}
					return -1
				}, publish.Data)
				decodedData, err := base64.StdEncoding.DecodeString(cleanData)
				if err != nil {
					// Skip invalid base64 data (some RIRs may have encoding issues)
					fmt.Printf("Skipping ROA %s due to base64 decoding error: %v\n", publish.URI, err)
					continue
				}

				// Get APNIC trust anchor
				ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, "APNIC", "rsync://rpki.apnic.net/repository/", "", "")
				if err != nil {
					fmt.Printf("Error getting APNIC trust anchor for ROA %s: %v\n", publish.URI, err)
					continue
				}

				vrps, err := i.roaProcessor.ExtractVRPsFromROAFile(ctx, decodedData, ta)
				if err != nil {
					fmt.Printf("Error processing ROA %s: %v\n", publish.URI, err)
					continue
				}

				fmt.Printf("Processed ROA %s: %d VRPs\n", publish.URI, len(vrps))
			}
		}
	}

	return nil
}

// ingestFromAPNICRsync ingests from APNIC rsync
func (i *Ingestor) ingestFromAPNICRsync(ctx context.Context) error {
	rsyncURI := "rsync://rpki.apnic.net/repository/"
	localPath := "/tmp/rpki-apnic"

	if err := i.rsyncClient.Sync(rsyncURI, localPath); err != nil {
		return err
	}

	fmt.Printf("Synced APNIC repository to %s\n", localPath)
	return nil
}

// ingestFromARINRRDP ingests from ARIN RRDP
func (i *Ingestor) ingestFromARINRRDP(ctx context.Context) error {
	notificationURL := "https://rrdp.arin.net/notification.xml"

	notification, err := i.rrdpClient.FetchNotification(notificationURL)
	if err != nil {
		return err
	}

	fmt.Printf("Fetched ARIN RRDP notification with serial %d\n", notification.Serial)

	if notification.Snapshot.URI != "" {
		snapshot, err := i.rrdpClient.FetchSnapshot(notification.Snapshot.URI)
		if err != nil {
			return err
		}

		fmt.Printf("Fetched ARIN snapshot with %d publishes\n", len(snapshot.Publishes))

		for _, publish := range snapshot.Publishes {
			if strings.HasSuffix(publish.URI, ".roa") {
				// Decode base64 data from RRDP (clean whitespace first)
				cleanData := strings.Map(func(r rune) rune {
					if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
						return r
					}
					return -1
				}, publish.Data)
				decodedData, err := base64.StdEncoding.DecodeString(cleanData)
				if err != nil {
					// Skip invalid base64 data (some RIRs may have encoding issues)
					fmt.Printf("Skipping ROA %s due to base64 decoding error: %v\n", publish.URI, err)
					continue
				}

				// Get ARIN trust anchor
				ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, "ARIN", "rsync://rpki.arin.net/repository/", "", "")
				if err != nil {
					fmt.Printf("Error getting ARIN trust anchor for ROA %s: %v\n", publish.URI, err)
					continue
				}

				vrps, err := i.roaProcessor.ExtractVRPsFromROAFile(ctx, decodedData, ta)
				if err != nil {
					fmt.Printf("Error processing ROA %s: %v\n", publish.URI, err)
					continue
				}

				fmt.Printf("Processed ROA %s: %d VRPs\n", publish.URI, len(vrps))
			}
		}
	}

	return nil
}

// ingestFromARINRsync ingests from ARIN rsync
func (i *Ingestor) ingestFromARINRsync(ctx context.Context) error {
	rsyncURI := "rsync://rpki.arin.net/repository/"
	localPath := "/tmp/rpki-arin"

	if err := i.rsyncClient.Sync(rsyncURI, localPath); err != nil {
		return err
	}

	fmt.Printf("Synced ARIN repository to %s\n", localPath)
	return nil
}

// ingestFromLACNICRRDP ingests from LACNIC RRDP
func (i *Ingestor) ingestFromLACNICRRDP(ctx context.Context) error {
	notificationURL := "https://rrdp.lacnic.net/rrdp/notification.xml"

	notification, err := i.rrdpClient.FetchNotification(notificationURL)
	if err != nil {
		return err
	}

	fmt.Printf("Fetched LACNIC RRDP notification with serial %d\n", notification.Serial)

	if notification.Snapshot.URI != "" {
		snapshot, err := i.rrdpClient.FetchSnapshot(notification.Snapshot.URI)
		if err != nil {
			return err
		}

		fmt.Printf("Fetched LACNIC snapshot with %d publishes\n", len(snapshot.Publishes))

		for _, publish := range snapshot.Publishes {
			if strings.HasSuffix(publish.URI, ".roa") {
				// Decode base64 data from RRDP (clean whitespace first)
				cleanData := strings.Map(func(r rune) rune {
					if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
						return r
					}
					return -1
				}, publish.Data)
				decodedData, err := base64.StdEncoding.DecodeString(cleanData)
				if err != nil {
					// Skip invalid base64 data (some RIRs may have encoding issues)
					fmt.Printf("Skipping ROA %s due to base64 decoding error: %v\n", publish.URI, err)
					continue
				}

				// Get LACNIC trust anchor
				ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, "LACNIC", "rsync://repository.lacnic.net/rpki/", "", "")
				if err != nil {
					fmt.Printf("Error getting LACNIC trust anchor for ROA %s: %v\n", publish.URI, err)
					continue
				}

				vrps, err := i.roaProcessor.ExtractVRPsFromROAFile(ctx, decodedData, ta)
				if err != nil {
					fmt.Printf("Error processing ROA %s: %v\n", publish.URI, err)
					continue
				}

				fmt.Printf("Processed ROA %s: %d VRPs\n", publish.URI, len(vrps))
			}
		}
	}

	return nil
}

// ingestFromLACNICRsync ingests from LACNIC rsync
func (i *Ingestor) ingestFromLACNICRsync(ctx context.Context) error {
	rsyncURI := "rsync://repository.lacnic.net/rpki/"
	localPath := "/tmp/rpki-lacnic"

	if err := i.rsyncClient.Sync(rsyncURI, localPath); err != nil {
		return err
	}

	fmt.Printf("Synced LACNIC repository to %s\n", localPath)
	return nil
}

// ingestFromCloudflareJSON ingests from Cloudflare RPKI JSON feed
func (i *Ingestor) ingestFromCloudflareJSON(ctx context.Context) error {
	url := "https://rpki.cloudflare.com/rpki.json"

	resp, err := i.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Parse and process Cloudflare data
	roas, vrps, err := i.processCloudflareData(ctx, body)
	if err != nil {
		return fmt.Errorf("failed to process Cloudflare data: %w", err)
	}

	fmt.Printf("Ingested from Cloudflare: %d ROAs, %d VRPs\n", len(roas), len(vrps))
	return nil
}

// CloudflareJSON represents the Cloudflare RPKI JSON structure
type CloudflareJSON struct {
	Roas []struct {
		ASN       int    `json:"asn"`
		Prefix    string `json:"prefix"`
		MaxLength int    `json:"maxLength"`
		TA        string `json:"ta"`
		Expires   int64  `json:"expires"`
	} `json:"roas"`
}

// processCloudflareData processes Cloudflare JSON data into ROAs and VRPs
func (i *Ingestor) processCloudflareData(ctx context.Context, rawData []byte) ([]*model.ROA, []*model.VRP, error) {
	var cfData CloudflareJSON
	if err := json.Unmarshal(rawData, &cfData); err != nil {
		return nil, nil, fmt.Errorf("failed to parse Cloudflare JSON: %w", err)
	}

	var roas []*model.ROA
	var vrps []*model.VRP

	for _, cfRoa := range cfData.Roas {
		// Get or create ASN (cached)
		asn, err := i.getOrCreateASNCached(ctx, cfRoa.ASN, fmt.Sprintf("AS%d", cfRoa.ASN), "")
		if err != nil {
			fmt.Printf("Error getting/creating ASN %d: %v\n", cfRoa.ASN, err)
			continue
		}

		// Get or create Prefix (cached)
		prefix, err := i.getOrCreatePrefixCached(ctx, cfRoa.Prefix, asn.ID, cfRoa.MaxLength)
		if err != nil {
			fmt.Printf("Error getting/creating prefix %s: %v\n", cfRoa.Prefix, err)
			continue
		}

		// Get trust anchor
		ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, "Cloudflare", "https://rpki.cloudflare.com/", "", "")
		if err != nil {
			fmt.Printf("Error getting trust anchor for Cloudflare: %v\n", err)
			continue
		}

		// Create ROA
		roa := &model.ROA{
			ID:        uuid.New().String(),
			ASNID:     asn.ID,
			PrefixID:  prefix.ID,
			MaxLength: cfRoa.MaxLength,
			NotBefore: time.Now(),
			NotAfter:  time.Unix(cfRoa.Expires, 0),
			Signature: "",
			TALID:     ta.ID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Insert ROA
		if err := i.dbClient.InsertROA(ctx, roa); err != nil {
			fmt.Printf("Error inserting ROA for AS%d %s: %v\n", cfRoa.ASN, cfRoa.Prefix, err)
			continue
		}

		// Generate VRPs from this ROA
		vrpList, err := i.roaProcessor.ProcessROA(roa, prefix)
		if err != nil {
			fmt.Printf("Error processing ROA for AS%d %s: %v\n", cfRoa.ASN, cfRoa.Prefix, err)
			continue
		}

		// Insert VRPs
		for _, vrp := range vrpList {
			vrp.ROAID = roa.ID
			if err := i.dbClient.InsertVRP(ctx, vrp); err != nil {
				fmt.Printf("Error inserting VRP: %v\n", err)
			}
		}

		roas = append(roas, roa)
		vrps = append(vrps, vrpList...)
	}

	return roas, vrps, nil
}

// RIPEData represents the RIPEstat data structure
type RIPEData struct {
	Data struct {
		RPKIValidation []struct {
			ASN       int    `json:"asn"`
			Prefix    string `json:"prefix"`
			MaxLength int    `json:"maxLength"`
			Validity  struct {
				Valid     bool  `json:"valid"`
				NotBefore int64 `json:"notBefore"`
				NotAfter  int64 `json:"notAfter"`
			} `json:"validity"`
		} `json:"rpki_validation"`
	} `json:"data"`
}

// processRIPEData processes RIPEstat data into ROAs and VRPs
func (i *Ingestor) processRIPEData(ctx context.Context, rawData []byte) ([]*model.ROA, []*model.VRP, error) {
	var ripeData RIPEData
	if err := json.Unmarshal(rawData, &ripeData); err != nil {
		// Try alternative parsing for different RIPE response format
		var altData map[string]interface{}
		if err := json.Unmarshal(rawData, &altData); err != nil {
			return nil, nil, fmt.Errorf("failed to parse RIPE data: %w", err)
		}
		// Process alternative format
		return i.processAlternativeRIPEData(ctx, altData)
	}

	var roas []*model.ROA
	var vrps []*model.VRP

	for _, validation := range ripeData.Data.RPKIValidation {
		if !validation.Validity.Valid {
			continue
		}

		// Get or create ASN (cached)
		asn, err := i.getOrCreateASNCached(ctx, validation.ASN, fmt.Sprintf("AS%d", validation.ASN), "")
		if err != nil {
			fmt.Printf("Error getting/creating ASN %d: %v\n", validation.ASN, err)
			continue
		}

		// Get or create Prefix (cached)
		prefix, err := i.getOrCreatePrefixCached(ctx, validation.Prefix, asn.ID, validation.MaxLength)
		if err != nil {
			fmt.Printf("Error getting/creating prefix %s: %v\n", validation.Prefix, err)
			continue
		}

		// Get RIPE trust anchor
		ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, "RIPE NCC", "rsync://rpki.ripe.net/repository/", "", "")
		if err != nil {
			fmt.Printf("Error getting RIPE trust anchor: %v\n", err)
			continue
		}

		// Create ROA
		roa := &model.ROA{
			ID:        uuid.New().String(),
			ASNID:     asn.ID,
			PrefixID:  prefix.ID,
			MaxLength: validation.MaxLength,
			NotBefore: time.Unix(validation.Validity.NotBefore, 0),
			NotAfter:  time.Unix(validation.Validity.NotAfter, 0),
			Signature: "",
			TALID:     ta.ID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Insert ROA
		if err := i.dbClient.InsertROA(ctx, roa); err != nil {
			fmt.Printf("Error inserting ROA for AS%d %s: %v\n", validation.ASN, validation.Prefix, err)
			continue
		}

		// Generate VRPs from this ROA
		vrpList, err := i.roaProcessor.ProcessROA(roa, prefix)
		if err != nil {
			fmt.Printf("Error processing ROA for AS%d %s: %v\n", validation.ASN, validation.Prefix, err)
			continue
		}

		// Set ROA ID on VRPs
		for _, vrp := range vrpList {
			vrp.ROAID = roa.ID
		}

		roas = append(roas, roa)
		vrps = append(vrps, vrpList...)

		// Batch insert VRPs for this ROA
		if err := i.dbClient.BatchInsertVRPs(ctx, vrpList); err != nil {
			fmt.Printf("Error batch inserting VRPs for ROA %s: %v\n", roa.ID, err)
		}
	}

	return roas, vrps, nil
}

// processAlternativeRIPEData processes alternative RIPE data format
func (i *Ingestor) processAlternativeRIPEData(ctx context.Context, data map[string]interface{}) ([]*model.ROA, []*model.VRP, error) {
	var roas []*model.ROA
	var vrps []*model.VRP

	// Try to extract RPKI validation data from alternative format
	if valData, ok := data["rpki_validation"].([]interface{}); ok {
		for _, item := range valData {
			if valMap, ok := item.(map[string]interface{}); ok {
				// Extract ASN
				asnNum := 0
				if asnVal, ok := valMap["asn"].(float64); ok {
					asnNum = int(asnVal)
				}

				// Extract prefix
				prefixStr := ""
				if prefixVal, ok := valMap["prefix"].(string); ok {
					prefixStr = prefixVal
				}

				// Extract max length
				maxLen := 0
				if maxLenVal, ok := valMap["maxLength"].(float64); ok {
					maxLen = int(maxLenVal)
				}

				if asnNum > 0 && prefixStr != "" {
					// Get or create ASN (cached)
					asn, err := i.getOrCreateASNCached(ctx, asnNum, fmt.Sprintf("AS%d", asnNum), "")
					if err != nil {
						fmt.Printf("Error getting/creating ASN %d: %v\n", asnNum, err)
						continue
					}

					// Get or create Prefix (cached)
					prefix, err := i.getOrCreatePrefixCached(ctx, prefixStr, asn.ID, maxLen)
					if err != nil {
						fmt.Printf("Error getting/creating prefix %s: %v\n", prefixStr, err)
						continue
					}

					// Get trust anchor
					ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, "RIPE NCC", "rsync://rpki.ripe.net/repository/", "", "")
					if err != nil {
						fmt.Printf("Error getting trust anchor for RIPE: %v\n", err)
						continue
					}

					roa := &model.ROA{
						ID:        uuid.New().String(),
						ASNID:     asn.ID,
						PrefixID:  prefix.ID,
						MaxLength: maxLen,
						NotBefore: time.Now(),
						NotAfter:  time.Now().Add(365 * 24 * time.Hour),
						Signature: "",
						TALID:     ta.ID,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}

					// Insert ROA
					if err := i.dbClient.InsertROA(ctx, roa); err != nil {
						fmt.Printf("Error inserting ROA for AS%d %s: %v\n", asnNum, prefixStr, err)
						continue
					}

					vrpList, err := i.roaProcessor.ProcessROA(roa, prefix)
					if err != nil {
						continue
					}

					// Set ROA ID on VRPs
					for _, vrp := range vrpList {
						vrp.ROAID = roa.ID
					}

					roas = append(roas, roa)
					vrps = append(vrps, vrpList...)

					// Batch insert VRPs for this ROA
					if err := i.dbClient.BatchInsertVRPs(ctx, vrpList); err != nil {
						fmt.Printf("Error batch inserting VRPs for ROA %s: %v\n", roa.ID, err)
					}
				}
			}
		}
	}

	return roas, vrps, nil
}

// IngestFromAllSources ingests from all configured sources concurrently
func (i *Ingestor) IngestFromAllSources(ctx context.Context) error {
	// Ensure trust anchors exist
	if err := i.ensureTrustAnchors(ctx); err != nil {
		return fmt.Errorf("failed to ensure trust anchors: %w", err)
	}

	// Create worker pool with limited concurrency (3 workers to avoid overwhelming sources)
	pool := NewWorkerPool(3, i)

	tasks := []Task{
		{Name: "RIPE", Fn: i.IngestFromRIPE},
		{Name: "AFRINIC", Fn: i.IngestFromAFRINIC},
		{Name: "APNIC", Fn: i.IngestFromAPNIC},
		{Name: "ARIN", Fn: i.IngestFromARIN},
		{Name: "LACNIC", Fn: i.IngestFromLACNIC},
		{Name: "Cloudflare", Fn: i.IngestFromCloudflare},
	}

	return pool.Execute(ctx, tasks)
}

// getOrCreateASNCached gets or creates an ASN with caching
func (i *Ingestor) getOrCreateASNCached(ctx context.Context, number int, name, country string) (*model.ASN, error) {
	// Check cache first
	if cached, ok := i.cache.asns.Load(number); ok {
		return cached.(*model.ASN), nil
	}

	// Not in cache, get/create from DB
	asn, err := i.dbClient.GetOrCreateASN(ctx, number, name, country)
	if err != nil {
		return nil, err
	}

	// Store in cache
	i.cache.asns.Store(number, asn)
	return asn, nil
}

// getOrCreatePrefixCached gets or creates a prefix with caching
func (i *Ingestor) getOrCreatePrefixCached(ctx context.Context, cidr string, asnID string, maxLength int) (*model.Prefix, error) {
	// Check cache first
	if cached, ok := i.cache.prefixes.Load(cidr); ok {
		return cached.(*model.Prefix), nil
	}

	// Not in cache, get from DB first
	prefix, err := i.dbClient.GetPrefixByCIDR(ctx, cidr)
	if err != nil {
		return nil, err
	}

	if prefix == nil {
		// Create new prefix
		prefix = &model.Prefix{
			ID:              uuid.New().String(),
			CIDR:            cidr,
			ASNID:           asnID,
			MaxLength:       sql.NullInt64{Int64: int64(maxLength), Valid: true},
			ValidationState: "UNKNOWN",
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		prefix, err = i.dbClient.UpsertPrefix(ctx, prefix)
		if err != nil {
			return nil, err
		}
	}

	// Store in cache
	i.cache.prefixes.Store(cidr, prefix)
	return prefix, nil
}

// getCircuitBreaker gets or creates a circuit breaker for a TA
func (i *Ingestor) getCircuitBreaker(taName string) *CircuitBreaker {
	cb, exists := i.circuitBreakers.Load(taName)
	if !exists {
		// Create new circuit breaker with default settings
		newCB := NewCircuitBreaker(3, 5*time.Minute) // 3 failures, 5 minute timeout
		cb, _ = i.circuitBreakers.LoadOrStore(taName, newCB)
	}
	return cb.(*CircuitBreaker)
}

// ingestFromRRDPWithFallback tries RRDP first, falls back to rsync
func (i *Ingestor) ingestFromRRDPWithFallback(ctx context.Context, taName, rrdpURL, rsyncURL string) error {
	// Try RRDP first
	fmt.Printf("→ Attempting RRDP ingestion for %s from %s\n", taName, rrdpURL)
	err := i.ingestFromRRDP(ctx, taName, rrdpURL)
	if err == nil {
		fmt.Printf("✓ Successfully ingested %s via RRDP\n", taName)
		return nil
	}

	fmt.Printf("⚠ RRDP failed for %s: %v\n", taName, err)
	fmt.Printf("→ Falling back to rsync for %s...\n", taName)

	// Fall back to rsync
	localPath := fmt.Sprintf("/tmp/rpki-%s", strings.ToLower(strings.ReplaceAll(taName, " ", "")))
	if err := i.rsyncClient.Sync(rsyncURL, localPath); err != nil {
		return fmt.Errorf("both RRDP and rsync failed for %s: RRDP error: %v, rsync error: %w", taName, err, err)
	}

	fmt.Printf("✓ Successfully synced %s repository to %s\n", taName, localPath)
	return i.processLocalRPKIFiles(ctx, taName, localPath)
}

// ingestFromRRDP ingests from RRDP with proper logging
func (i *Ingestor) ingestFromRRDP(ctx context.Context, taName, notificationURL string) error {
	fmt.Printf("→ Fetching RRDP notification for %s from %s\n", taName, notificationURL)

	notification, err := i.rrdpClient.FetchNotification(notificationURL)
	if err != nil {
		return fmt.Errorf("failed to fetch notification: %w", err)
	}

	fmt.Printf("  ✓ Got notification (serial: %d, session: %s)\n",
		notification.Serial, notification.SessionID)

	if notification.Snapshot.URI == "" {
		return fmt.Errorf("no snapshot URI in notification")
	}

	fmt.Printf("→ Downloading snapshot from %s\n", notification.Snapshot.URI)
	fmt.Printf("  (This may take several minutes for large snapshots...)\n")

	snapshot, err := i.rrdpClient.FetchSnapshot(notification.Snapshot.URI)
	if err != nil {
		return fmt.Errorf("failed to fetch snapshot: %w", err)
	}

	fmt.Printf("  ✓ Downloaded snapshot with %d objects\n", len(snapshot.Publishes))

	// Get trust anchor with correct URI
	var taURI string
	switch taName {
	case "RIPE NCC":
		taURI = "rsync://rpki.ripe.net/repository/"
	case "AFRINIC":
		taURI = "rsync://rpki.afrinic.net/repository/"
	case "APNIC":
		taURI = "rsync://rpki.apnic.net/repository/"
	case "ARIN":
		taURI = "rsync://rpki.arin.net/repository/"
	case "LACNIC":
		taURI = "rsync://repository.lacnic.net/rpki/"
	default:
		taURI = fmt.Sprintf("rsync://%s/", strings.ToLower(taName))
	}

	ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, taName, taURI, "", "")
	if err != nil {
		return fmt.Errorf("failed to get trust anchor: %w", err)
	}

	// Process snapshot publishes
	for _, publish := range snapshot.Publishes {
		if strings.HasSuffix(publish.URI, ".roa") {
			// Use improved base64 decoding
			decodedData, err := i.cleanBase64(publish.Data)
			if err != nil {
				fmt.Printf("Skipping ROA %s due to base64 decoding error: %v\n", publish.URI, err)
				continue
			}

			vrps, err := i.roaProcessor.ExtractVRPsFromROAFile(ctx, decodedData, ta)
			if err != nil {
				fmt.Printf("Error processing ROA %s: %v\n", publish.URI, err)
				continue
			}

			fmt.Printf("Processed ROA %s: %d VRPs\n", publish.URI, len(vrps))
		}
	}

	return nil
}

// processLocalRPKIFiles processes files downloaded via rsync
func (i *Ingestor) processLocalRPKIFiles(ctx context.Context, taName, localPath string) error {
	// TODO: Implement local file processing for rsync fallback
	// This would walk the directory tree and process .roa files
	fmt.Printf("Note: Local file processing for %s not yet implemented\n", taName)
	return nil
}

// cleanBase64 properly cleans base64 data with all whitespace handling
func (i *Ingestor) cleanBase64(data string) ([]byte, error) {
	// Remove ALL whitespace and control characters
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return -1
		}
		return r
	}, data)

	// Remove any remaining non-base64 characters
	cleaned = strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '+' || r == '/' || r == '=' {
			return r
		}
		return -1
	}, cleaned)

	return base64.StdEncoding.DecodeString(cleaned)
}

// ingestWithRetry wraps an ingestion function with retry logic and circuit breaker
func (i *Ingestor) ingestWithRetry(ctx context.Context, name string, fn func(context.Context) error) error {
	cb := i.getCircuitBreaker(name)

	// Check if circuit breaker allows execution
	if !cb.CanExecute() {
		fmt.Printf("Circuit breaker open for %s, skipping ingestion\n", name)
		return fmt.Errorf("circuit breaker open for %s", name)
	}

	var err error
	for attempt := 0; attempt <= i.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: base delay * 2^(attempt-1)
			backoff := time.Duration(attempt) * i.config.RetryBackoff
			fmt.Printf("Retrying %s ingestion in %v (attempt %d/%d)\n", name, backoff, attempt, i.config.MaxRetries)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				cb.RecordFailure()
				return ctx.Err()
			}
		}

		err = fn(ctx)
		if err == nil {
			cb.RecordSuccess()
			return nil
		}

		fmt.Printf("Attempt %d failed for %s: %v\n", attempt+1, name, err)
	}

	// All attempts failed
	cb.RecordFailure()

	if i.config.SkipOnFailure {
		fmt.Printf("Skipping %s after %d failed attempts\n", name, i.config.MaxRetries+1)
		return nil
	}

	return fmt.Errorf("all %d attempts failed for %s: %w", i.config.MaxRetries+1, name, err)
}

// Helper function to parse ASN from string
func parseASN(asnStr string) (int, error) {
	// Remove AS prefix if present
	asnStr = strings.TrimPrefix(strings.ToUpper(asnStr), "AS")
	return strconv.Atoi(asnStr)
}

// Helper function to validate CIDR
func validateCIDR(cidr string) error {
	_, _, err := net.ParseCIDR(cidr)
	return err
}

// cleanBase64Data removes all non-base64 characters including whitespace
func cleanBase64Data(data string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
			return r
		}
		return -1
	}, data)
}
