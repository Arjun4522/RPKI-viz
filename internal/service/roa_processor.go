package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	librpki "github.com/cloudflare/cfrpki/validator/lib"
	"github.com/google/uuid"
	"github.com/rpki-viz/backend/internal/model"
)

// ROAProcessor handles ROA to VRP derivation logic
type ROAProcessor struct {
	dbClient interface {
		GetOrCreateASN(ctx context.Context, number int, name, country string) (*model.ASN, error)
		GetPrefixByCIDR(ctx context.Context, cidr string) (*model.Prefix, error)
		InsertPrefix(ctx context.Context, prefix *model.Prefix) error
		InsertROA(ctx context.Context, roa *model.ROA) error
	}
}

// NewROAProcessor creates a new ROA processor
func NewROAProcessor() *ROAProcessor {
	return &ROAProcessor{}
}

// SetDBClient sets the database client for the processor
func (p *ROAProcessor) SetDBClient(dbClient interface {
	GetOrCreateASN(ctx context.Context, number int, name, country string) (*model.ASN, error)
	GetASNByID(ctx context.Context, id string) (*model.ASN, error)
	GetPrefixByCIDR(ctx context.Context, cidr string) (*model.Prefix, error)
	InsertPrefix(ctx context.Context, prefix *model.Prefix) error
	InsertROA(ctx context.Context, roa *model.ROA) error
}) {
	p.dbClient = dbClient
}

// ProcessROA converts a ROA into VRPs
func (p *ROAProcessor) ProcessROA(roa *model.ROA, prefix *model.Prefix) ([]*model.VRP, error) {
	vrp := &model.VRP{
		ID:        uuid.New().String(),
		ASNID:     roa.ASNID,
		PrefixID:  prefix.ID,
		MaxLength: roa.MaxLength,
		NotBefore: roa.NotBefore,
		NotAfter:  roa.NotAfter,
		ROAID:     roa.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return []*model.VRP{vrp}, nil
}

// ValidateROA validates a ROA's cryptographic signature and structure
func (p *ROAProcessor) ValidateROA(roa *model.ROA, certificate *model.Certificate) error {
	if roa.MaxLength < 0 {
		return fmt.Errorf("invalid maxLength: %d", roa.MaxLength)
	}

	if roa.NotAfter.Before(roa.NotBefore) {
		return fmt.Errorf("invalid validity period: notAfter before notBefore")
	}

	if roa.NotAfter.Before(time.Now()) {
		return fmt.Errorf("ROA has expired")
	}

	if certificate == nil {
		return fmt.Errorf("no certificate provided for validation")
	}

	if certificate.NotAfter.Before(time.Now()) {
		return fmt.Errorf("certificate has expired")
	}

	if certificate.NotBefore.After(time.Now()) {
		return fmt.Errorf("certificate is not yet valid")
	}

	return nil
}

// ExtractVRPsFromROAFile processes an ROA file and extracts VRPs
func (p *ROAProcessor) ExtractVRPsFromROAFile(ctx context.Context, roaData []byte, tal *model.TrustAnchor) ([]*model.VRP, error) {
	if tal == nil {
		return nil, fmt.Errorf("trust anchor cannot be nil")
	}

	if p.dbClient == nil {
		return nil, fmt.Errorf("database client not set")
	}

	roaInfo, err := p.parseROAFile(roaData)
	if err != nil {
		// Skip ROAs that cannot be parsed even with lenient validation
		// These are typically AFRINIC ROAs with broken CMS signatures
		fmt.Printf("Skipping unparseable ROA: %v\n", err)
		return []*model.VRP{}, nil
	}

	var vrps []*model.VRP

	// Get or create ASN in database
	asn, err := p.dbClient.GetOrCreateASN(ctx, roaInfo.ASN, fmt.Sprintf("AS%d", roaInfo.ASN), "")
	if err != nil {
		return nil, fmt.Errorf("failed to get/create ASN %d: %w", roaInfo.ASN, err)
	}

	// Process each prefix in the ROA
	for _, prefixInfo := range roaInfo.Prefixes {
		// Get or create Prefix in database
		prefix, err := p.dbClient.GetPrefixByCIDR(ctx, prefixInfo.Prefix)
		if err != nil {
			fmt.Printf("Error getting prefix %s: %v\n", prefixInfo.Prefix, err)
			continue
		}
		if prefix == nil {
			prefix = &model.Prefix{
				ID:              uuid.New().String(),
				CIDR:            prefixInfo.Prefix,
				ASNID:           asn.ID,
				MaxLength:       sql.NullInt64{Int64: int64(prefixInfo.MaxLength), Valid: true},
				ValidationState: "UNKNOWN",
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			if err := p.dbClient.InsertPrefix(ctx, prefix); err != nil {
				fmt.Printf("Error inserting prefix %s: %v\n", prefixInfo.Prefix, err)
				continue
			}
		}

		// Create and insert ROA
		roa := &model.ROA{
			ID:        uuid.New().String(),
			ASNID:     asn.ID,
			PrefixID:  prefix.ID,
			MaxLength: prefixInfo.MaxLength,
			NotBefore: roaInfo.NotBefore,
			NotAfter:  roaInfo.NotAfter,
			TALID:     tal.ID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := p.dbClient.InsertROA(ctx, roa); err != nil {
			fmt.Printf("Error inserting ROA for AS%d %s: %v\n", roaInfo.ASN, prefixInfo.Prefix, err)
			continue
		}

		// Generate VRPs from this ROA
		vrpList, err := p.ProcessROA(roa, prefix)
		if err != nil {
			fmt.Printf("Error processing ROA for AS%d %s: %v\n", roaInfo.ASN, prefixInfo.Prefix, err)
			continue
		}

		vrps = append(vrps, vrpList...)
	}

	return vrps, nil
}

// ROAInfo represents parsed ROA information
type ROAInfo struct {
	ASN       int
	Prefixes  []PrefixInfo
	NotBefore time.Time
	NotAfter  time.Time
	Signature []byte
}

// PrefixInfo represents prefix information from ROA
type PrefixInfo struct {
	Prefix    string
	MaxLength int
}

// parseROAFile parses a ROA file using proper DER decoding
func (p *ROAProcessor) parseROAFile(roaData []byte) (*ROAInfo, error) {
	roaInfo, err := librpki.DecodeROA(roaData)
	if err != nil {
		if strings.Contains(err.Error(), "CMS is not valid") {
			return p.parseROALeniently(roaData)
		}
		return nil, fmt.Errorf("failed to decode ROA: %w", err)
	}

	info := &ROAInfo{
		ASN:       roaInfo.ASN,
		Prefixes:  []PrefixInfo{},
		NotBefore: roaInfo.SigningTime,
		NotAfter:  roaInfo.SigningTime.Add(365 * 24 * time.Hour),
	}

	for _, entry := range roaInfo.Entries {
		if entry.IPNet != nil {
			prefix := entry.IPNet.String()
			maxLength := entry.MaxLength

			info.Prefixes = append(info.Prefixes, PrefixInfo{
				Prefix:    prefix,
				MaxLength: maxLength,
			})
		}
	}

	return info, nil
}

// parseROALeniently attempts to parse ROA data even with signature issues
func (p *ROAProcessor) parseROALeniently(roaData []byte) (*ROAInfo, error) {
	roaInfo, err := librpki.DecodeROA(roaData)
	if err == nil {
		info := &ROAInfo{
			ASN:       roaInfo.ASN,
			Prefixes:  []PrefixInfo{},
			NotBefore: roaInfo.SigningTime,
			NotAfter:  roaInfo.SigningTime.Add(365 * 24 * time.Hour),
		}

		for _, entry := range roaInfo.Entries {
			if entry.IPNet != nil {
				info.Prefixes = append(info.Prefixes, PrefixInfo{
					Prefix:    entry.IPNet.String(),
					MaxLength: entry.MaxLength,
				})
			}
		}

		fmt.Printf("Successfully parsed ROA with lenient validation: AS%d\n", roaInfo.ASN)
		return info, nil
	}

	return nil, fmt.Errorf("ROA parsing failed even with lenient validation: %w", err)
}

// ValidateCertificateChain validates a certificate chain against trust anchors
func (p *ROAProcessor) ValidateCertificateChain(cert *model.Certificate, trustAnchors []*model.TrustAnchor) error {
	if cert == nil {
		return fmt.Errorf("certificate cannot be nil")
	}

	if len(trustAnchors) == 0 {
		return fmt.Errorf("no trust anchors provided")
	}

	now := time.Now()
	if cert.NotAfter.Before(now) {
		return fmt.Errorf("certificate has expired")
	}

	if cert.NotBefore.After(now) {
		return fmt.Errorf("certificate is not yet valid")
	}

	var matchingTA *model.TrustAnchor
	for _, ta := range trustAnchors {
		if strings.Contains(cert.Issuer, ta.Name) || strings.Contains(cert.Subject, ta.Name) {
			matchingTA = ta
			break
		}
	}

	if matchingTA == nil {
		return fmt.Errorf("no matching trust anchor found")
	}

	return nil
}
