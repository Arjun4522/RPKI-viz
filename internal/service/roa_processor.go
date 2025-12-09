package service

import (
	"fmt"
	"net"
	"strings"
	"time"

	librpki "github.com/cloudflare/cfrpki/validator/lib"
	"github.com/google/uuid"
	"github.com/rpki-viz/backend/internal/model"
)

// ROAProcessor handles ROA to VRP derivation logic
type ROAProcessor struct {
	// Dependencies can be added here (e.g., database, logger)
}

// NewROAProcessor creates a new ROA processor
func NewROAProcessor() *ROAProcessor {
	return &ROAProcessor{}
}

// ProcessROA converts a ROA into VRPs
func (p *ROAProcessor) ProcessROA(roa *model.ROA, prefix *model.Prefix) ([]*model.VRP, error) {
	var vrps []*model.VRP

	// Parse the CIDR prefix
	_, ipNet, err := net.ParseCIDR(prefix.CIDR)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR prefix %s: %w", prefix.CIDR, err)
	}

	// Calculate the maximum prefix length
	maxLength := roa.MaxLength
	if maxLength == 0 {
		// If maxLength is not specified, use the prefix length
		maxLength = prefix.MaxLength
	}

	// Generate VRPs for all prefixes from the ROA prefix length to maxLength
	prefixLength, _ := ipNet.Mask.Size()

	// Validate maxLength
	if maxLength < prefixLength {
		maxLength = prefixLength
	}

	// Generate VRPs for each possible prefix length
	for length := prefixLength; length <= maxLength; length++ {
		// Generate all possible prefixes at this length
		subnets := p.generateSubnetsForLength(ipNet, length)

		for _, subnet := range subnets {
			_ = subnet // Mark as used to avoid compiler warning
			vrp := &model.VRP{
				ID:        uuid.New().String(),
				ASNID:     roa.ASNID,
				PrefixID:  prefix.ID,
				MaxLength: maxLength,
				NotBefore: roa.NotBefore,
				NotAfter:  roa.NotAfter,
				ROAID:     roa.ID,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			vrps = append(vrps, vrp)
		}
	}

	return vrps, nil
}

// generateSubnetsForLength generates all possible subnets of a specific length from a base prefix
func (p *ROAProcessor) generateSubnetsForLength(baseNet *net.IPNet, targetLength int) []string {
	var subnets []string

	baseLength, _ := baseNet.Mask.Size()
	if targetLength < baseLength {
		return subnets
	}

	// Calculate the number of subnets
	numSubnets := 1 << (targetLength - baseLength)

	// Get the base IP as a 16-byte array
	baseIP := baseNet.IP.To16()
	if baseIP == nil {
		return subnets
	}

	// Generate each subnet
	for i := 0; i < numSubnets; i++ {
		// Calculate the new IP
		newIP := make(net.IP, len(baseIP))
		copy(newIP, baseIP)

		// Add the subnet offset
		byteOffset := (baseLength + i*(1<<(targetLength-baseLength))) / 8
		bitOffset := (baseLength + i*(1<<(targetLength-baseLength))) % 8

		if byteOffset < len(newIP) {
			newIP[byteOffset] |= byte(i << (8 - bitOffset))
		}

		// Create the new CIDR
		subnet := &net.IPNet{
			IP:   newIP,
			Mask: net.CIDRMask(targetLength, len(newIP)*8),
		}

		subnets = append(subnets, subnet.String())
	}

	return subnets
}

// ValidateROA validates a ROA's cryptographic signature and structure
func (p *ROAProcessor) ValidateROA(roa *model.ROA, certificate *model.Certificate) error {
	// Basic validation checks
	if roa.MaxLength < 0 {
		return fmt.Errorf("invalid maxLength: %d", roa.MaxLength)
	}

	if roa.NotAfter.Before(roa.NotBefore) {
		return fmt.Errorf("invalid validity period: notAfter before notBefore")
	}

	if roa.NotAfter.Before(time.Now()) {
		return fmt.Errorf("ROA has expired")
	}

	// Certificate chain validation
	if certificate == nil {
		return fmt.Errorf("no certificate provided for validation")
	}

	// Validate certificate validity
	if certificate.NotAfter.Before(time.Now()) {
		return fmt.Errorf("certificate has expired")
	}

	if certificate.NotBefore.After(time.Now()) {
		return fmt.Errorf("certificate is not yet valid")
	}

	// TODO: Implement full cryptographic validation
	// This would involve:
	// 1. Verifying the certificate chain against trust anchors
	// 2. Checking the ROA signature using the certificate's public key
	// 3. Validating the ROA structure against RFC 6482
	// 4. Checking certificate revocation status

	return nil
}

// ExtractVRPsFromROAFile processes an ROA file and extracts VRPs
func (p *ROAProcessor) ExtractVRPsFromROAFile(roaData []byte, tal *model.TrustAnchor) ([]*model.VRP, error) {
	if tal == nil {
		return nil, fmt.Errorf("trust anchor cannot be nil")
	}

	// Parse ROA data (DER format expected)
	roaInfo, err := p.parseROAFile(roaData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ROA file: %w", err)
	}

	var vrps []*model.VRP

	// Create ASN
	asn := &model.ASN{
		ID:        uuid.New().String(),
		Number:    roaInfo.ASN,
		Name:      fmt.Sprintf("AS%d", roaInfo.ASN),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Process each prefix in the ROA
	for _, prefixInfo := range roaInfo.Prefixes {
		// Create Prefix
		prefix := &model.Prefix{
			ID:        uuid.New().String(),
			CIDR:      prefixInfo.Prefix,
			ASNID:     asn.ID,
			MaxLength: prefixInfo.MaxLength,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Create ROA
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
	// Use the cfrpki library to properly decode the ROA file
	rpkiROA, err := librpki.DecodeROA(roaData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ROA: %w", err)
	}

	info := &ROAInfo{
		ASN:       rpkiROA.ASN,
		Prefixes:  []PrefixInfo{},
		NotBefore: rpkiROA.SigningTime,
		NotAfter:  rpkiROA.SigningTime.Add(365 * 24 * time.Hour), // Default 1 year validity
	}

	// Extract prefixes from ROA entries
	for _, entry := range rpkiROA.Entries {
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

// ValidateCertificateChain validates a certificate chain against trust anchors
func (p *ROAProcessor) ValidateCertificateChain(cert *model.Certificate, trustAnchors []*model.TrustAnchor) error {
	if cert == nil {
		return fmt.Errorf("certificate cannot be nil")
	}

	if len(trustAnchors) == 0 {
		return fmt.Errorf("no trust anchors provided")
	}

	// Check certificate validity
	now := time.Now()
	if cert.NotAfter.Before(now) {
		return fmt.Errorf("certificate has expired")
	}

	if cert.NotBefore.After(now) {
		return fmt.Errorf("certificate is not yet valid")
	}

	// Find matching trust anchor
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

	// TODO: Implement full certificate chain validation
	// This would involve:
	// 1. Verifying the certificate signature against the trust anchor
	// 2. Checking the certificate chain integrity
	// 3. Validating certificate extensions
	// 4. Checking revocation status

	return nil
}
