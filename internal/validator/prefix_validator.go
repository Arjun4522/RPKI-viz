package validator

import (
	"fmt"
	"net"
	"time"

	"github.com/rpki-viz/backend/internal/model"
)

// PrefixValidator handles prefix validation logic
type PrefixValidator struct {
	// Dependencies can be added here (e.g., database, logger)
}

// NewPrefixValidator creates a new prefix validator
func NewPrefixValidator() *PrefixValidator {
	return &PrefixValidator{}
}

// ValidationResult represents the result of prefix validation
type ValidationResult struct {
	State  model.ValidationState
	Reason string
}

// ValidatePrefix validates a prefix against RPKI data following RFC 6811
// vrps should be all VRPs for the given ASN
func (v *PrefixValidator) ValidatePrefix(asn int, prefixStr string, vrps []*model.VRP) *ValidationResult {
	// Parse the input prefix
	inputPrefix, inputBits := v.parsePrefixToIPNet(prefixStr)
	if inputPrefix == nil {
		return &ValidationResult{
			State:  model.Unknown,
			Reason: "Invalid prefix format",
		}
	}

	if len(vrps) == 0 {
		return &ValidationResult{
			State:  model.NotFound,
			Reason: "No VRPs found for this ASN",
		}
	}

	now := time.Now()
	var validVRPs []*model.VRP
	var expiredVRPs []*model.VRP

	// Check each VRP for coverage and validity
	for _, vrp := range vrps {
		// Check if VRP is currently valid
		if !vrp.NotBefore.Before(now) || !vrp.NotAfter.After(now) {
			expiredVRPs = append(expiredVRPs, vrp)
			continue
		}

		// Check if VRP covers the input prefix
		// Note: This is simplified - in production you'd need to look up the VRP's prefix CIDR
		// For now, we assume VRPs are pre-filtered to relevant prefixes
		covers := v.vrpCoversPrefix(vrp, inputPrefix, inputBits)
		if covers {
			validVRPs = append(validVRPs, vrp)
		}
	}

	// Apply RPKI validation logic
	if len(validVRPs) > 0 {
		return &ValidationResult{
			State:  model.Valid,
			Reason: fmt.Sprintf("Prefix is covered by %d valid VRP(s)", len(validVRPs)),
		}
	}

	if len(expiredVRPs) > 0 {
		return &ValidationResult{
			State:  model.Invalid,
			Reason: "Prefix was previously covered by VRPs but they have expired",
		}
	}

	return &ValidationResult{
		State:  model.NotFound,
		Reason: "No VRP covers this prefix for the given ASN",
	}
}

// ValidatePrefixAgainstVRPs validates a prefix against a specific set of VRPs
func (v *PrefixValidator) ValidatePrefixAgainstVRPs(asn int, prefix string, vrps []*model.VRP) *ValidationResult {
	return v.ValidatePrefix(asn, prefix, vrps)
}

// ValidatePrefixWithReason provides detailed validation reasoning
func (v *PrefixValidator) ValidatePrefixWithReason(asn int, prefix string, vrps []*model.VRP) *ValidationResult {
	result := v.ValidatePrefix(asn, prefix, vrps)

	// Add additional reasoning based on the result
	switch result.State {
	case model.Valid:
		result.Reason = "Prefix is covered by valid RPKI data"
	case model.Invalid:
		if result.Reason == "All matching VRPs have expired" {
			result.Reason = "Prefix was previously valid but all RPKI attestations have expired"
		} else {
			result.Reason = "Prefix validation failed - no valid RPKI coverage"
		}
	case model.NotFound:
		result.Reason = "No RPKI data found for this prefix and ASN combination"
	case model.Unknown:
		result.Reason = "Unable to determine prefix validation status"
	}

	return result
}

// parsePrefixToIPNet parses a CIDR string and returns IP network and prefix length
func (v *PrefixValidator) parsePrefixToIPNet(prefixStr string) (*net.IPNet, int) {
	_, ipNet, err := net.ParseCIDR(prefixStr)
	if err != nil {
		return nil, 0
	}
	inputBits, _ := ipNet.Mask.Size()
	return ipNet, inputBits
}

// vrpCoversPrefix checks if a VRP covers the given prefix according to RFC 6811
// Note: This is a simplified implementation. In production, you'd need to:
// 1. Look up the VRP's prefix CIDR from the database
// 2. Parse both prefixes as IP networks
// 3. Check containment and maxLength constraints
func (v *PrefixValidator) vrpCoversPrefix(vrp *model.VRP, inputPrefix *net.IPNet, inputBits int) bool {
	// For now, we assume that VRPs passed to this validator are already
	// filtered to be relevant to the input prefix. In a full implementation,
	// this method would:
	//
	// 1. Get the VRP's prefix from database using vrp.PrefixID
	// 2. Parse VRP prefix as CIDR
	// 3. Check if inputPrefix is contained within VRP's prefix network
	// 4. Verify maxLength constraints (VRP maxLength >= input prefix length)
	//
	// For this simplified version, we check maxLength and mark inputPrefix as used

	_ = inputPrefix // Mark as used to avoid compiler warning
	return vrp.MaxLength >= inputBits
}
