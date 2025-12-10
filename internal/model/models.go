package model

import (
	"database/sql"
	"time"
)

// ASN represents an Autonomous System Number
type ASN struct {
	ID        string    `json:"id"`
	Number    int       `json:"number"`
	Name      string    `json:"name"`
	Country   string    `json:"country"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Prefix represents an IP prefix in CIDR notation
type Prefix struct {
	ID              string        `json:"id"`
	CIDR            string        `json:"cidr"`
	ASNID           string        `json:"asnId"`
	MaxLength       sql.NullInt64 `json:"maxLength"`
	ExpiresAt       sql.NullTime  `json:"expiresAt"`
	ValidationState string        `json:"validationState"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

// TrustAnchor represents a Trust Anchor Locator
type TrustAnchor struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URI       string    `json:"uri"`
	RSAKey    string    `json:"rsaKey"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Certificate represents an X.509 certificate
type Certificate struct {
	ID                     string    `json:"id"`
	Subject                string    `json:"subject"`
	Issuer                 string    `json:"issuer"`
	SerialNumber           string    `json:"serialNumber"`
	NotBefore              time.Time `json:"notBefore"`
	NotAfter               time.Time `json:"notAfter"`
	SubjectKeyIdentifier   string    `json:"subjectKeyIdentifier"`
	AuthorityKeyIdentifier string    `json:"authorityKeyIdentifier"`
	CRLDistributionPoints  []string  `json:"crlDistributionPoints"`
	AuthorityInfoAccess    []string  `json:"authorityInfoAccess"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

// ROA represents a Route Origin Authorization
type ROA struct {
	ID        string    `json:"id"`
	ASNID     string    `json:"asnId"`
	PrefixID  string    `json:"prefixId"`
	MaxLength int       `json:"maxLength"`
	NotBefore time.Time `json:"notBefore"`
	NotAfter  time.Time `json:"notAfter"`
	Signature string    `json:"signature"`
	TALID     string    `json:"talId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// VRP represents a Validated ROA Payload
type VRP struct {
	ID        string    `json:"id"`
	ASNID     string    `json:"asnId"`
	PrefixID  string    `json:"prefixId"`
	MaxLength int       `json:"maxLength"`
	NotBefore time.Time `json:"notBefore"`
	NotAfter  time.Time `json:"notAfter"`
	ROAID     string    `json:"roaId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ValidationState represents the validation state of a prefix
type ValidationState string

const (
	Valid    ValidationState = "VALID"
	Invalid  ValidationState = "INVALID"
	NotFound ValidationState = "NOT_FOUND"
	Unknown  ValidationState = "UNKNOWN"
)

// RIRStat represents statistics for a Regional Internet Registry
type RIRStat struct {
	RIR              string `json:"rir"`
	TotalROAs        int    `json:"totalROAs"`
	TotalVRPs        int    `json:"totalVRPs"`
	ValidPrefixes    int    `json:"validPrefixes"`
	InvalidPrefixes  int    `json:"invalidPrefixes"`
	NotFoundPrefixes int    `json:"notFoundPrefixes"`
}

// GlobalSummary represents global summary metrics
type GlobalSummary struct {
	TotalASNs        int       `json:"totalASNs"`
	TotalPrefixes    int       `json:"totalPrefixes"`
	TotalROAs        int       `json:"totalROAs"`
	TotalVRPs        int       `json:"totalVRPs"`
	ValidPrefixes    int       `json:"validPrefixes"`
	InvalidPrefixes  int       `json:"invalidPrefixes"`
	NotFoundPrefixes int       `json:"notFoundPrefixes"`
	RIRStats         []RIRStat `json:"rirStats"`
}

// ValidationResponse represents the response for a prefix validation
type ValidationResponse struct {
	ASN    int             `json:"asn"`
	Prefix string          `json:"prefix"`
	State  ValidationState `json:"state"`
	Reason string          `json:"reason,omitempty"`
}
