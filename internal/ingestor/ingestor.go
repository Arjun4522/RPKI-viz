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
		GetASNByID(ctx context.Context, id string) (*model.ASN, error)
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
	GetASNByID(ctx context.Context, id string) (*model.ASN, error)
	GetOrCreateTrustAnchor(ctx context.Context, name, uri, rsaKey, sha256 string) (*model.TrustAnchor, error)
	GetPrefixByCIDR(ctx context.Context, cidr string) (*model.Prefix, error)
	InsertPrefix(ctx context.Context, prefix *model.Prefix) error
	InsertROA(ctx context.Context, roa *model.ROA) error
	InsertVRP(ctx context.Context, vrp *model.VRP) error
}) *Ingestor {
	roaProc := service.NewROAProcessor()
	roaProc.SetDBClient(dbClient)

	return &Ingestor{
		rrdpClient:   rrdp.NewClient(),
		rsyncClient:  rsync.NewClient(),
		roaProcessor: roaProc,
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

// IngestFromAFRINIC ingests data from AFRINIC sources
func (i *Ingestor) IngestFromAFRINIC(ctx context.Context) error {
	// Ingest from AFRINIC RRDP
	if err := i.ingestFromAFRINICRRDP(ctx); err != nil {
		return fmt.Errorf("failed to ingest from AFRINIC RRDP: %w", err)
	}

	// Ingest from AFRINIC rsync
	if err := i.ingestFromAFRINICRsync(ctx); err != nil {
		return fmt.Errorf("failed to ingest from AFRINIC rsync: %w", err)
	}

	return nil
}

// IngestFromAPNIC ingests data from APNIC sources
func (i *Ingestor) IngestFromAPNIC(ctx context.Context) error {
	// Ingest from APNIC RRDP
	if err := i.ingestFromAPNICRRDP(ctx); err != nil {
		return fmt.Errorf("failed to ingest from APNIC RRDP: %w", err)
	}

	// Ingest from APNIC rsync
	if err := i.ingestFromAPNICRsync(ctx); err != nil {
		return fmt.Errorf("failed to ingest from APNIC rsync: %w", err)
	}

	return nil
}

// IngestFromARIN ingests data from ARIN sources
func (i *Ingestor) IngestFromARIN(ctx context.Context) error {
	// Ingest from ARIN RRDP
	if err := i.ingestFromARINRRDP(ctx); err != nil {
		return fmt.Errorf("failed to ingest from ARIN RRDP: %w", err)
	}

	// Ingest from ARIN rsync
	if err := i.ingestFromARINRsync(ctx); err != nil {
		return fmt.Errorf("failed to ingest from ARIN rsync: %w", err)
	}

	return nil
}

// IngestFromLACNIC ingests data from LACNIC sources
func (i *Ingestor) IngestFromLACNIC(ctx context.Context) error {
	// Ingest from LACNIC RRDP
	if err := i.ingestFromLACNICRRDP(ctx); err != nil {
		return fmt.Errorf("failed to ingest from LACNIC RRDP: %w", err)
	}

	// Ingest from LACNIC rsync
	if err := i.ingestFromLACNICRsync(ctx); err != nil {
		return fmt.Errorf("failed to ingest from LACNIC rsync: %w", err)
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

				// Insert VRPs into database, ensuring ASN IDs are valid
				for _, vrp := range vrps {
					// Verify the ASN exists before inserting VRP
					existingASN, err := i.dbClient.GetASNByID(ctx, vrp.ASNID)
					if err != nil || existingASN == nil {
						fmt.Printf("VRP references non-existent ASN ID %s, skipping\n", vrp.ASNID)
						continue
					}

					if err := i.dbClient.InsertVRP(ctx, vrp); err != nil {
						fmt.Printf("Error inserting VRP: %v\n", err)
					}
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

				// Insert VRPs into database, ensuring ASN IDs are valid
				for _, vrp := range vrps {
					// Verify the ASN exists before inserting VRP
					existingASN, err := i.dbClient.GetASNByID(ctx, vrp.ASNID)
					if err != nil || existingASN == nil {
						fmt.Printf("VRP references non-existent ASN ID %s, skipping\n", vrp.ASNID)
						continue
					}

					if err := i.dbClient.InsertVRP(ctx, vrp); err != nil {
						fmt.Printf("Error inserting VRP: %v\n", err)
					}
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

				// Insert VRPs into database, ensuring ASN IDs are valid
				for _, vrp := range vrps {
					// Verify the ASN exists before inserting VRP
					existingASN, err := i.dbClient.GetASNByID(ctx, vrp.ASNID)
					if err != nil || existingASN == nil {
						fmt.Printf("VRP references non-existent ASN ID %s, skipping\n", vrp.ASNID)
						continue
					}

					if err := i.dbClient.InsertVRP(ctx, vrp); err != nil {
						fmt.Printf("Error inserting VRP: %v\n", err)
					}
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

				// Insert VRPs into database, ensuring ASN IDs are valid
				for _, vrp := range vrps {
					// Verify the ASN exists before inserting VRP
					existingASN, err := i.dbClient.GetASNByID(ctx, vrp.ASNID)
					if err != nil || existingASN == nil {
						fmt.Printf("VRP references non-existent ASN ID %s, skipping\n", vrp.ASNID)
						continue
					}

					if err := i.dbClient.InsertVRP(ctx, vrp); err != nil {
						fmt.Printf("Error inserting VRP: %v\n", err)
					}
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

				// Insert VRPs into database, ensuring ASN IDs are valid
				for _, vrp := range vrps {
					// Verify the ASN exists before inserting VRP
					existingASN, err := i.dbClient.GetASNByID(ctx, vrp.ASNID)
					if err != nil || existingASN == nil {
						fmt.Printf("VRP references non-existent ASN ID %s, skipping\n", vrp.ASNID)
						continue
					}

					if err := i.dbClient.InsertVRP(ctx, vrp); err != nil {
						fmt.Printf("Error inserting VRP: %v\n", err)
					}
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
				MaxLength:       sql.NullInt64{Int64: int64(cfRoa.MaxLength), Valid: true},
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
				MaxLength:       sql.NullInt64{Int64: int64(validation.MaxLength), Valid: true},
				ValidationState: "UNKNOWN",
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			if err := i.dbClient.InsertPrefix(ctx, prefix); err != nil {
				fmt.Printf("Error inserting prefix %s: %v\n", validation.Prefix, err)
				continue
			}
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
							MaxLength:       sql.NullInt64{Int64: int64(maxLen), Valid: true},
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

// IngestFromAllSources ingests from all configured sources concurrently
func (i *Ingestor) IngestFromAllSources(ctx context.Context) error {
	// Ensure trust anchors exist
	if err := i.ensureTrustAnchors(ctx); err != nil {
		return fmt.Errorf("failed to ensure trust anchors: %w", err)
	}

	sources := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"RIPE", i.IngestFromRIPE},
		{"AFRINIC", i.IngestFromAFRINIC},
		{"APNIC", i.IngestFromAPNIC},
		{"ARIN", i.IngestFromARIN},
		{"LACNIC", i.IngestFromLACNIC},
		{"Cloudflare", i.IngestFromCloudflare},
	}

	// Use goroutines for concurrent processing
	var wg sync.WaitGroup
	errChan := make(chan error, len(sources))

	for _, source := range sources {
		wg.Add(1)
		go func(name string, fn func(context.Context) error) {
			defer wg.Done()
			fmt.Printf("Starting ingestion from %s...\n", name)
			if err := fn(ctx); err != nil {
				fmt.Printf("Error ingesting from %s: %v\n", name, err)
				errChan <- fmt.Errorf("%s: %w", name, err)
			} else {
				fmt.Printf("Completed ingestion from %s\n", name)
			}
		}(source.name, source.fn)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Collect any errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("ingestion completed with errors: %v", errors)
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
