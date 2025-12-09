package validator

import (
	"net"
	"strconv"
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

// ValidatePrefix validates a prefix against RPKI data
func (v *PrefixValidator) ValidatePrefix(asn int, prefix string, vrps []*model.VRP) *ValidationResult {
	// Parse the target prefix
	targetIP, targetNet, err := net.ParseCIDR(prefix)
	if err != nil {
		return &ValidationResult{
			State:  model.Invalid,
			Reason: "Invalid CIDR notation",
		}
	}

	// Find matching VRPs for this ASN and prefix
	var matchingVRPs []*model.VRP
	for _, vrp := range vrps {
		if vrp.ASNID == strconv.Itoa(asn) {
			// Parse VRP prefix
			_, vrpNet, err := net.ParseCIDR(vrp.PrefixID)
			if err != nil {
				continue
			}

			// Check if the target prefix is covered by this VRP
			if vrpNet.Contains(targetIP) {
				// Check prefix length constraints
				targetLength, _ := targetNet.Mask.Size()
				if targetLength <= vrp.MaxLength {
					matchingVRPs = append(matchingVRPs, vrp)
				}
			}
		}
	}

	if len(matchingVRPs) == 0 {
		return &ValidationResult{
			State:  model.NotFound,
			Reason: "No matching VRPs found",
		}
	}

	// Check if any matching VRP is currently valid
	now := time.Now()
	for _, vrp := range matchingVRPs {
		if vrp.NotBefore.Before(now) && vrp.NotAfter.After(now) {
			return &ValidationResult{
				State:  model.Valid,
				Reason: "Prefix is covered by valid VRP",
			}
		}
	}

	// All matching VRPs are expired
	return &ValidationResult{
		State:  model.Invalid,
		Reason: "All matching VRPs have expired",
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
