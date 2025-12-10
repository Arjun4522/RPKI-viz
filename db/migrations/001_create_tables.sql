-- RPKI Visualization Platform Database Schema

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Autonomous System Numbers
CREATE TABLE asns (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    number BIGINT NOT NULL UNIQUE,
    name VARCHAR(255),
    country CHAR(2),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_asns_number ON asns(number);
CREATE INDEX idx_asns_country ON asns(country);

-- IP Prefixes
CREATE TABLE prefixes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    cidr CIDR NOT NULL UNIQUE,
    asn_id UUID REFERENCES asns(id),
    max_length INTEGER,
    expires_at TIMESTAMP WITH TIME ZONE,
    validation_state VARCHAR(20) NOT NULL DEFAULT 'UNKNOWN',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_prefixes_cidr ON prefixes(cidr);
CREATE INDEX idx_prefixes_asn ON prefixes(asn_id);
CREATE INDEX idx_prefixes_validation ON prefixes(validation_state);

-- Trust Anchor Locators
CREATE TABLE trust_anchors (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    uri TEXT NOT NULL,
    rsa_key TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_trust_anchors_name ON trust_anchors(name);

-- Certificates
CREATE TABLE certificates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    subject TEXT NOT NULL,
    issuer TEXT NOT NULL,
    serial_number TEXT NOT NULL,
    not_before TIMESTAMP WITH TIME ZONE NOT NULL,
    not_after TIMESTAMP WITH TIME ZONE NOT NULL,
    subject_key_identifier TEXT,
    authority_key_identifier TEXT,
    crl_distribution_points TEXT[],
    authority_info_access TEXT[],
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Route Origin Authorizations
CREATE TABLE roas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    asn_id UUID NOT NULL REFERENCES asns(id),
    prefix_id UUID NOT NULL REFERENCES prefixes(id),
    max_length INTEGER NOT NULL,
    not_before TIMESTAMP WITH TIME ZONE NOT NULL,
    not_after TIMESTAMP WITH TIME ZONE NOT NULL,
    signature TEXT NOT NULL,
    tal_id UUID NOT NULL REFERENCES trust_anchors(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_roas_asn ON roas(asn_id);
CREATE INDEX idx_roas_prefix ON roas(prefix_id);
CREATE INDEX idx_roas_tal ON roas(tal_id);
CREATE INDEX idx_roas_expiry ON roas(not_after);

-- Validated ROA Payloads
CREATE TABLE vrps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    asn_id UUID NOT NULL REFERENCES asns(id),
    prefix_id UUID NOT NULL REFERENCES prefixes(id),
    max_length INTEGER NOT NULL,
    not_before TIMESTAMP WITH TIME ZONE NOT NULL,
    not_after TIMESTAMP WITH TIME ZONE NOT NULL,
    roa_id UUID NOT NULL REFERENCES roas(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vrps_asn ON vrps(asn_id);
CREATE INDEX idx_vrps_prefix ON vrps(prefix_id);
CREATE INDEX idx_vrps_roa ON vrps(roa_id);
CREATE INDEX idx_vrps_expiry ON vrps(not_after);

-- Certificate chains (many-to-many relationship)
CREATE TABLE certificate_chains (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    parent_cert_id UUID NOT NULL REFERENCES certificates(id),
    child_cert_id UUID NOT NULL REFERENCES certificates(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_certificate_chains_parent ON certificate_chains(parent_cert_id);
CREATE INDEX idx_certificate_chains_child ON certificate_chains(child_cert_id);

-- ROA-Certificate relationship
CREATE TABLE roa_certificates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    roa_id UUID NOT NULL REFERENCES roas(id),
    certificate_id UUID NOT NULL REFERENCES certificates(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_roa_certificates_roa ON roa_certificates(roa_id);
CREATE INDEX idx_roa_certificates_cert ON roa_certificates(certificate_id);

-- RIR Statistics
CREATE TABLE rir_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    rir VARCHAR(50) NOT NULL,
    total_roas INTEGER NOT NULL DEFAULT 0,
    total_vrps INTEGER NOT NULL DEFAULT 0,
    valid_prefixes INTEGER NOT NULL DEFAULT 0,
    invalid_prefixes INTEGER NOT NULL DEFAULT 0,
    not_found_prefixes INTEGER NOT NULL DEFAULT 0,
    recorded_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rir_stats_rir ON rir_stats(rir);
CREATE INDEX idx_rir_stats_recorded ON rir_stats(recorded_at);

-- Create updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers for automatic updated_at updates
CREATE TRIGGER update_asns_updated_at BEFORE UPDATE ON asns
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_prefixes_updated_at BEFORE UPDATE ON prefixes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_trust_anchors_updated_at BEFORE UPDATE ON trust_anchors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_certificates_updated_at BEFORE UPDATE ON certificates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_roas_updated_at BEFORE UPDATE ON roas
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_vrps_updated_at BEFORE UPDATE ON vrps
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();