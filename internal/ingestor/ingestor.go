package ingestor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rpki-viz/backend/internal/model"
	"github.com/rpki-viz/backend/internal/service"
	"github.com/rpki-viz/backend/pkg/rrdp"
	"github.com/rpki-viz/backend/pkg/rsync"
)

// Ingestor handles data ingestion from various RPKI sources
type Ingestor struct {
	rrdpClient   *rrdp.Client
	rsyncClient  *rsync.Client
	roaProcessor *service.ROAProcessor
	httpClient   *http.Client
	dbClient     interface {
		GetOrCreateASN(ctx context.Context, number int, name, country string) (*model.ASN, error)
		GetOrCreateTrustAnchor(ctx context.Context, name, uri, rsaKey, sha256 string) (*model.TrustAnchor, error)
		GetPrefixByCIDR(ctx context.Context, cidr string) (*model.Prefix, error)
		InsertPrefix(ctx context.Context, prefix *model.Prefix) error
		InsertROA(ctx context.Context, roa *model.ROA) error
		InsertVRP(ctx context.Context, vrp *model.VRP) error
	}
}

// NewIngestor creates a new ingestor
func NewIngestor(dbClient interface {
	GetOrCreateASN(ctx context.Context, number int, name, country string) (*model.ASN, error)
	GetOrCreateTrustAnchor(ctx context.Context, name, uri, rsaKey, sha256 string) (*model.TrustAnchor, error)
	GetPrefixByCIDR(ctx context.Context, cidr string) (*model.Prefix, error)
	InsertPrefix(ctx context.Context, prefix *model.Prefix) error
	InsertROA(ctx context.Context, roa *model.ROA) error
	InsertVRP(ctx context.Context, vrp *model.VRP) error
}) *Ingestor {
	return &Ingestor{
		rrdpClient:   rrdp.NewClient(),
		rsyncClient:  rsync.NewClient(),
		roaProcessor: service.NewROAProcessor(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		dbClient: dbClient,
	}
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
			name:   "Cloudflare",
			uri:    "https://rpki.cloudflare.com/",
			rsaKey: "",
			sha256: "",
		},
		{
			name:   "NLnet Labs",
			uri:    "https://nlnetlabs.nl/",
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
	// Skip RIPEstat as it requires parameters and doesn't provide bulk data
	// Ingest from RIPE RRDP
	if err := i.ingestFromRIPERRDP(ctx); err != nil {
		return fmt.Errorf("failed to ingest from RIPE RRDP: %w", err)
	}

	// Ingest from RIPE rsync
	if err := i.ingestFromRIPERsync(ctx); err != nil {
		return fmt.Errorf("failed to ingest from RIPE rsync: %w", err)
	}

	return nil
}

// IngestFromCloudflare ingests data from Cloudflare sources
func (i *Ingestor) IngestFromCloudflare(ctx context.Context) error {
	// Ingest from Cloudflare RPKI JSON feed
	if err := i.ingestFromCloudflareJSON(ctx); err != nil {
		return fmt.Errorf("failed to ingest from Cloudflare JSON: %w", err)
	}

	return nil
}

// IngestFromRoutinator ingests data from NLnet Labs Routinator
func (i *Ingestor) IngestFromRoutinator(ctx context.Context) error {
	// Ingest from Routinator HTTP API
	if err := i.ingestFromRoutinatorAPI(ctx); err != nil {
		return fmt.Errorf("failed to ingest from Routinator API: %w", err)
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

		// Process snapshot data to extract ROAs
		for _, publish := range snapshot.Publishes {
			if strings.HasSuffix(publish.URI, ".roa") {
				vrps, err := i.roaProcessor.ExtractVRPsFromROAFile([]byte(publish.Data), nil)
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

// ingestFromRoutinatorAPI ingests from Routinator HTTP API
func (i *Ingestor) ingestFromRoutinatorAPI(ctx context.Context) error {
	url := "http://localhost:9556/json" // Default Routinator port

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

	// Parse and process Routinator data
	roas, vrps, err := i.processRoutinatorData(ctx, body)
	if err != nil {
		return fmt.Errorf("failed to process Routinator data: %w", err)
	}

	fmt.Printf("Ingested from Routinator: %d ROAs, %d VRPs\n", len(roas), len(vrps))
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
		// Get or create ASN
		asn, err := i.dbClient.GetOrCreateASN(ctx, cfRoa.ASN, fmt.Sprintf("AS%d", cfRoa.ASN), "")
		if err != nil {
			fmt.Printf("Error getting/creating ASN %d: %v\n", cfRoa.ASN, err)
			continue
		}

		// Get or create Prefix
		prefix, err := i.dbClient.GetPrefixByCIDR(ctx, cfRoa.Prefix)
		if err != nil {
			fmt.Printf("Error getting prefix %s: %v\n", cfRoa.Prefix, err)
			continue
		}
		if prefix == nil {
			prefix = &model.Prefix{
				ID:              uuid.New().String(),
				CIDR:            cfRoa.Prefix,
				ASNID:           asn.ID,
				MaxLength:       cfRoa.MaxLength,
				ValidationState: "UNKNOWN",
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			if err := i.dbClient.InsertPrefix(ctx, prefix); err != nil {
				fmt.Printf("Error inserting prefix %s: %v\n", cfRoa.Prefix, err)
				continue
			}
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

// RoutinatorJSON represents the Routinator JSON structure
type RoutinatorJSON struct {
	Roas []struct {
		ASN         int    `json:"asn"`
		Prefix      string `json:"prefix"`
		MaxLength   int    `json:"maxLength"`
		TrustAnchor string `json:"trustAnchor"`
		Validity    struct {
			NotBefore int64 `json:"notBefore"`
			NotAfter  int64 `json:"notAfter"`
		} `json:"validity"`
	} `json:"roas"`
}

// processRoutinatorData processes Routinator JSON data into ROAs and VRPs
func (i *Ingestor) processRoutinatorData(ctx context.Context, rawData []byte) ([]*model.ROA, []*model.VRP, error) {
	var rtData RoutinatorJSON
	if err := json.Unmarshal(rawData, &rtData); err != nil {
		return nil, nil, fmt.Errorf("failed to parse Routinator JSON: %w", err)
	}

	var roas []*model.ROA
	var vrps []*model.VRP

	for _, rtRoa := range rtData.Roas {
		// Get or create ASN
		asn, err := i.dbClient.GetOrCreateASN(ctx, rtRoa.ASN, fmt.Sprintf("AS%d", rtRoa.ASN), "")
		if err != nil {
			fmt.Printf("Error getting/creating ASN %d: %v\n", rtRoa.ASN, err)
			continue
		}

		// Get or create Prefix
		prefix, err := i.dbClient.GetPrefixByCIDR(ctx, rtRoa.Prefix)
		if err != nil {
			fmt.Printf("Error getting prefix %s: %v\n", rtRoa.Prefix, err)
			continue
		}
		if prefix == nil {
			prefix = &model.Prefix{
				ID:              uuid.New().String(),
				CIDR:            rtRoa.Prefix,
				ASNID:           asn.ID,
				MaxLength:       rtRoa.MaxLength,
				ValidationState: "UNKNOWN",
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			if err := i.dbClient.InsertPrefix(ctx, prefix); err != nil {
				fmt.Printf("Error inserting prefix %s: %v\n", rtRoa.Prefix, err)
				continue
			}
		}

		// Get trust anchor
		ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, "NLnet Labs", "https://nlnetlabs.nl/", "", "")
		if err != nil {
			fmt.Printf("Error getting trust anchor for Routinator: %v\n", err)
			continue
		}

		// Create ROA
		roa := &model.ROA{
			ID:        uuid.New().String(),
			ASNID:     asn.ID,
			PrefixID:  prefix.ID,
			MaxLength: rtRoa.MaxLength,
			NotBefore: time.Unix(rtRoa.Validity.NotBefore, 0),
			NotAfter:  time.Unix(rtRoa.Validity.NotAfter, 0),
			Signature: "",
			TALID:     ta.ID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Insert ROA
		if err := i.dbClient.InsertROA(ctx, roa); err != nil {
			fmt.Printf("Error inserting ROA for AS%d %s: %v\n", rtRoa.ASN, rtRoa.Prefix, err)
			continue
		}

		// Generate VRPs from this ROA
		vrpList, err := i.roaProcessor.ProcessROA(roa, prefix)
		if err != nil {
			fmt.Printf("Error processing ROA for AS%d %s: %v\n", rtRoa.ASN, rtRoa.Prefix, err)
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

		// Get or create ASN
		asn, err := i.dbClient.GetOrCreateASN(ctx, validation.ASN, fmt.Sprintf("AS%d", validation.ASN), "")
		if err != nil {
			fmt.Printf("Error getting/creating ASN %d: %v\n", validation.ASN, err)
			continue
		}

		// Get or create Prefix
		prefix, err := i.dbClient.GetPrefixByCIDR(ctx, validation.Prefix)
		if err != nil {
			fmt.Printf("Error getting prefix %s: %v\n", validation.Prefix, err)
			continue
		}
		if prefix == nil {
			prefix = &model.Prefix{
				ID:              uuid.New().String(),
				CIDR:            validation.Prefix,
				ASNID:           asn.ID,
				MaxLength:       validation.MaxLength,
				ValidationState: "UNKNOWN",
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			if err := i.dbClient.InsertPrefix(ctx, prefix); err != nil {
				fmt.Printf("Error inserting prefix %s: %v\n", validation.Prefix, err)
				continue
			}
		}

		// Get trust anchor
		ta, err := i.dbClient.GetOrCreateTrustAnchor(ctx, "RIPE NCC", "rsync://rpki.ripe.net/repository/", "", "")
		if err != nil {
			fmt.Printf("Error getting trust anchor for RIPE: %v\n", err)
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
					// Get or create ASN
					asn, err := i.dbClient.GetOrCreateASN(ctx, asnNum, fmt.Sprintf("AS%d", asnNum), "")
					if err != nil {
						fmt.Printf("Error getting/creating ASN %d: %v\n", asnNum, err)
						continue
					}

					// Get or create Prefix
					prefix, err := i.dbClient.GetPrefixByCIDR(ctx, prefixStr)
					if err != nil {
						fmt.Printf("Error getting prefix %s: %v\n", prefixStr, err)
						continue
					}
					if prefix == nil {
						prefix = &model.Prefix{
							ID:              uuid.New().String(),
							CIDR:            prefixStr,
							ASNID:           asn.ID,
							MaxLength:       maxLen,
							ValidationState: "UNKNOWN",
							CreatedAt:       time.Now(),
							UpdatedAt:       time.Now(),
						}
						if err := i.dbClient.InsertPrefix(ctx, prefix); err != nil {
							fmt.Printf("Error inserting prefix %s: %v\n", prefixStr, err)
							continue
						}
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
			}
		}
	}

	return roas, vrps, nil
}

// IngestFromAllSources ingests from all configured sources
func (i *Ingestor) IngestFromAllSources(ctx context.Context) error {
	// Ensure trust anchors exist
	if err := i.ensureTrustAnchors(ctx); err != nil {
		return fmt.Errorf("failed to ensure trust anchors: %w", err)
	}

	sources := []func(context.Context) error{
		// i.IngestFromRIPE,
		i.IngestFromCloudflare,
		// i.IngestFromRoutinator,
	}

	for _, source := range sources {
		if err := source(ctx); err != nil {
			// Log error but continue with other sources
			fmt.Printf("Error ingesting from source: %v\n", err)
		}
	}

	return nil
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
