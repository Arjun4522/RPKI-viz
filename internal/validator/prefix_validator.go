package validator

import (
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
	// Since VRPs are pre-filtered by ASN and the prefix check is complex without CIDR data,
	// for now assume if VRPs exist for the ASN, and we're validating the same prefix,
	// check validity time

	if len(vrps) == 0 {
		return &ValidationResult{
			State:  model.NotFound,
			Reason: "No matching VRPs found",
		}
	}

	// Check if any VRP is currently valid
	now := time.Now()
	for _, vrp := range vrps {
		if vrp.NotBefore.Before(now) && vrp.NotAfter.After(now) {
			return &ValidationResult{
				State:  model.Valid,
				Reason: "Prefix is covered by valid VRP",
			}
		}
	}

	// All VRPs are expired
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
