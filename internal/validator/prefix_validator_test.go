package validator

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rpki-viz/backend/internal/model"
)

func TestNewPrefixValidator(t *testing.T) {
	validator := NewPrefixValidator()
	if validator == nil {
		t.Fatal("NewPrefixValidator returned nil")
	}
}

func TestValidatePrefix_InvalidPrefix(t *testing.T) {
	validator := NewPrefixValidator()
	vrps := []*model.VRP{}

	result := validator.ValidatePrefix(123, "invalid-prefix", vrps)

	if result.State != model.Unknown {
		t.Errorf("Expected Unknown state for invalid prefix, got %v", result.State)
	}
	if result.Reason == "" {
		t.Error("Expected non-empty reason for invalid prefix")
	}
}

func TestValidatePrefix_NoVRPs(t *testing.T) {
	validator := NewPrefixValidator()
	vrps := []*model.VRP{}

	result := validator.ValidatePrefix(123, "192.168.1.0/24", vrps)

	if result.State != model.NotFound {
		t.Errorf("Expected NotFound state for no VRPs, got %v", result.State)
	}
	if result.Reason != "No VRPs found for this ASN" {
		t.Errorf("Unexpected reason: %s", result.Reason)
	}
}

func TestValidatePrefix_ValidVRP(t *testing.T) {
	validator := NewPrefixValidator()

	// Create a valid VRP
	now := time.Now()
	vrp := &model.VRP{
		ID:        uuid.New().String(),
		ASNID:     "asn-123",
		PrefixID:  "prefix-1",
		MaxLength: 24,
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(1 * time.Hour),
	}

	vrps := []*model.VRP{vrp}

	result := validator.ValidatePrefix(123, "192.168.1.0/24", vrps)

	if result.State != model.Valid {
		t.Errorf("Expected Valid state for valid VRP, got %v", result.State)
	}
	if !contains(result.Reason, "covered by") {
		t.Errorf("Expected coverage message, got: %s", result.Reason)
	}
}

func TestValidatePrefix_ExpiredVRP(t *testing.T) {
	validator := NewPrefixValidator()

	// Create an expired VRP
	now := time.Now()
	vrp := &model.VRP{
		ID:        uuid.New().String(),
		ASNID:     "asn-123",
		PrefixID:  "prefix-1",
		MaxLength: 24,
		NotBefore: now.Add(-2 * time.Hour),
		NotAfter:  now.Add(-1 * time.Hour), // Expired
	}

	vrps := []*model.VRP{vrp}

	result := validator.ValidatePrefix(123, "192.168.1.0/24", vrps)

	if result.State != model.Invalid {
		t.Errorf("Expected Invalid state for expired VRP, got %v", result.State)
	}
	if !contains(result.Reason, "expired") {
		t.Errorf("Expected expiration message, got: %s", result.Reason)
	}
}

func TestValidatePrefix_FutureVRP(t *testing.T) {
	validator := NewPrefixValidator()

	// Create a future VRP (not yet valid)
	now := time.Now()
	vrp := &model.VRP{
		ID:        uuid.New().String(),
		ASNID:     "asn-123",
		PrefixID:  "prefix-1",
		MaxLength: 24,
		NotBefore: now.Add(1 * time.Hour), // Future
		NotAfter:  now.Add(2 * time.Hour),
	}

	vrps := []*model.VRP{vrp}

	result := validator.ValidatePrefix(123, "192.168.1.0/24", vrps)

	if result.State != model.Invalid {
		t.Errorf("Expected Invalid state for future VRP, got %v", result.State)
	}
}

func TestValidatePrefix_MultipleVRPs(t *testing.T) {
	validator := NewPrefixValidator()

	now := time.Now()

	// Create multiple VRPs - some valid, some expired
	vrps := []*model.VRP{
		{
			ID:        uuid.New().String(),
			ASNID:     "asn-123",
			PrefixID:  "prefix-1",
			MaxLength: 24,
			NotBefore: now.Add(-1 * time.Hour),
			NotAfter:  now.Add(1 * time.Hour),
		},
		{
			ID:        uuid.New().String(),
			ASNID:     "asn-123",
			PrefixID:  "prefix-2",
			MaxLength: 24,
			NotBefore: now.Add(-2 * time.Hour),
			NotAfter:  now.Add(-1 * time.Hour), // Expired
		},
	}

	result := validator.ValidatePrefix(123, "192.168.1.0/24", vrps)

	if result.State != model.Valid {
		t.Errorf("Expected Valid state when at least one VRP is valid, got %v", result.State)
	}
}

func TestValidatePrefix_MaxLengthCheck(t *testing.T) {
	validator := NewPrefixValidator()

	now := time.Now()

	// Test with /25 prefix but VRP only allows /24 maxLength
	vrp := &model.VRP{
		ID:        uuid.New().String(),
		ASNID:     "asn-123",
		PrefixID:  "prefix-1",
		MaxLength: 24, // Only allows up to /24
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(1 * time.Hour),
	}

	vrps := []*model.VRP{vrp}

	// This should fail because /25 > maxLength of 24
	result := validator.ValidatePrefix(123, "192.168.1.0/25", vrps)

	if result.State != model.NotFound {
		t.Errorf("Expected NotFound state for prefix longer than maxLength, got %v", result.State)
	}
}

func TestValidatePrefixWithReason(t *testing.T) {
	validator := NewPrefixValidator()

	// Test with no VRPs
	result := validator.ValidatePrefixWithReason(123, "192.168.1.0/24", []*model.VRP{})

	if result.State != model.NotFound {
		t.Errorf("Expected NotFound state, got %v", result.State)
	}
	if result.Reason != "No RPKI data found for this prefix and ASN combination" {
		t.Errorf("Unexpected reason: %s", result.Reason)
	}
}

func TestParsePrefixToIPNet(t *testing.T) {
	validator := NewPrefixValidator()

	// Test valid CIDR
	ipNet, bits := validator.parsePrefixToIPNet("192.168.1.0/24")
	if ipNet == nil {
		t.Error("Expected valid IPNet for valid CIDR")
	}
	if bits != 24 {
		t.Errorf("Expected 24 bits, got %d", bits)
	}

	// Test invalid CIDR
	ipNet, bits = validator.parsePrefixToIPNet("invalid")
	if ipNet != nil {
		t.Error("Expected nil IPNet for invalid CIDR")
	}
	if bits != 0 {
		t.Errorf("Expected 0 bits for invalid CIDR, got %d", bits)
	}
}

func TestVrpCoversPrefix(t *testing.T) {
	validator := NewPrefixValidator()

	// Create a VRP with maxLength 24
	vrp := &model.VRP{
		MaxLength: 24,
	}

	// Test with /24 prefix (should pass)
	inputPrefix, inputBits := validator.parsePrefixToIPNet("192.168.1.0/24")
	covers := validator.vrpCoversPrefix(vrp, inputPrefix, inputBits)
	if !covers {
		t.Error("Expected /24 prefix to be covered by maxLength 24")
	}

	// Test with /25 prefix (should fail)
	inputPrefix, inputBits = validator.parsePrefixToIPNet("192.168.1.0/25")
	covers = validator.vrpCoversPrefix(vrp, inputPrefix, inputBits)
	if covers {
		t.Error("Expected /25 prefix to not be covered by maxLength 24")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
