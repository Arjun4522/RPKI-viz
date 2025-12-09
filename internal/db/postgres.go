package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/rpki-viz/backend/internal/model"
)

// PostgresClient handles PostgreSQL database operations
type PostgresClient struct {
	db *sql.DB
}

// NewPostgresClient creates a new PostgreSQL client
func NewPostgresClient(databaseURL string) (*PostgresClient, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresClient{db: db}, nil
}

// Close closes the database connection
func (c *PostgresClient) Close() error {
	return c.db.Close()
}

// HealthCheck checks if the database connection is healthy
func (c *PostgresClient) HealthCheck(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// GetASNByNumber retrieves an ASN by its number
func (c *PostgresClient) GetASNByNumber(ctx context.Context, number int) (*model.ASN, error) {
	query := `
		SELECT id, number, name, country, created_at, updated_at
		FROM asns
		WHERE number = $1
	`

	var asn model.ASN
	err := c.db.QueryRowContext(ctx, query, number).Scan(
		&asn.ID, &asn.Number, &asn.Name, &asn.Country, &asn.CreatedAt, &asn.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ASN by number: %w", err)
	}

	return &asn, nil
}

// GetASNs retrieves ASNs with pagination and filtering
func (c *PostgresClient) GetASNs(ctx context.Context, first, offset int, orderBy, filter interface{}) ([]*model.ASN, error) {
	query := `
		SELECT id, number, name, country, created_at, updated_at
		FROM asns
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 0

	// Add filter conditions if provided
	if filter != nil {
		if f, ok := filter.(map[string]interface{}); ok {
			if country, ok := f["country"].(string); ok && country != "" {
				argCount++
				query += fmt.Sprintf(" AND country = $%d", argCount)
				args = append(args, country)
			}
			if number, ok := f["number"].(int); ok && number > 0 {
				argCount++
				query += fmt.Sprintf(" AND number = $%d", argCount)
				args = append(args, number)
			}
		}
	}

	// Add ordering
	if orderBy != nil {
		if ob, ok := orderBy.(map[string]interface{}); ok {
			if field, ok := ob["field"].(string); ok {
				if direction, ok := ob["direction"].(string); ok {
					// Validate field and direction to prevent SQL injection
					validFields := map[string]bool{"number": true, "name": true, "country": true, "created_at": true}
					validDirections := map[string]bool{"ASC": true, "DESC": true}

					if validFields[field] && validDirections[direction] {
						query += fmt.Sprintf(" ORDER BY %s %s", field, direction)
					}
				}
			}
		}
	}

	// Add pagination
	if first > 0 {
		argCount++
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, first)
	}

	if offset > 0 {
		argCount++
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, offset)
	}

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query ASNs: %w", err)
	}
	defer rows.Close()

	var asns []*model.ASN
	for rows.Next() {
		var asn model.ASN
		err := rows.Scan(
			&asn.ID, &asn.Number, &asn.Name, &asn.Country, &asn.CreatedAt, &asn.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ASN row: %w", err)
		}
		asns = append(asns, &asn)
	}

	return asns, rows.Err()
}

// GetPrefixByCIDR retrieves a prefix by its CIDR notation
func (c *PostgresClient) GetPrefixByCIDR(ctx context.Context, cidr string) (*model.Prefix, error) {
	query := `
		SELECT p.id, p.cidr, p.asn_id, p.max_length, p.expires_at, p.validation_state, p.created_at, p.updated_at
		FROM prefixes p
		WHERE p.cidr = $1
	`

	var prefix model.Prefix
	err := c.db.QueryRowContext(ctx, query, cidr).Scan(
		&prefix.ID, &prefix.CIDR, &prefix.ASNID, &prefix.MaxLength,
		&prefix.ExpiresAt, &prefix.ValidationState, &prefix.CreatedAt, &prefix.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get prefix by CIDR: %w", err)
	}

	return &prefix, nil
}

// GetPrefixes retrieves prefixes with pagination and filtering
func (c *PostgresClient) GetPrefixes(ctx context.Context, first, offset int, orderBy, filter interface{}) ([]*model.Prefix, error) {
	query := `
		SELECT p.id, p.cidr, p.asn_id, p.max_length, p.expires_at, p.validation_state, p.created_at, p.updated_at
		FROM prefixes p
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 0

	// Add filter conditions if provided
	if filter != nil {
		if f, ok := filter.(map[string]interface{}); ok {
			if asnID, ok := f["asnId"].(string); ok && asnID != "" {
				argCount++
				query += fmt.Sprintf(" AND p.asn_id = $%d", argCount)
				args = append(args, asnID)
			}
			if validationState, ok := f["validationState"].(string); ok && validationState != "" {
				argCount++
				query += fmt.Sprintf(" AND p.validation_state = $%d", argCount)
				args = append(args, validationState)
			}
		}
	}

	// Add ordering
	if orderBy != nil {
		if ob, ok := orderBy.(map[string]interface{}); ok {
			if field, ok := ob["field"].(string); ok {
				if direction, ok := ob["direction"].(string); ok {
					// Validate field and direction to prevent SQL injection
					validFields := map[string]bool{"cidr": true, "asn_id": true, "validation_state": true, "created_at": true}
					validDirections := map[string]bool{"ASC": true, "DESC": true}

					if validFields[field] && validDirections[direction] {
						query += fmt.Sprintf(" ORDER BY p.%s %s", field, direction)
					}
				}
			}
		}
	}

	// Add pagination
	if first > 0 {
		argCount++
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, first)
	}

	if offset > 0 {
		argCount++
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, offset)
	}

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query prefixes: %w", err)
	}
	defer rows.Close()

	var prefixes []*model.Prefix
	for rows.Next() {
		var prefix model.Prefix
		err := rows.Scan(
			&prefix.ID, &prefix.CIDR, &prefix.ASNID, &prefix.MaxLength,
			&prefix.ExpiresAt, &prefix.ValidationState, &prefix.CreatedAt, &prefix.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan prefix row: %w", err)
		}
		prefixes = append(prefixes, &prefix)
	}

	return prefixes, rows.Err()
}

// GetROAByID retrieves a ROA by its ID
func (c *PostgresClient) GetROAByID(ctx context.Context, id string) (*model.ROA, error) {
	query := `
		SELECT id, asn_id, prefix_id, max_length, not_before, not_after, signature, tal_id, created_at, updated_at
		FROM roas
		WHERE id = $1
	`

	var roa model.ROA
	err := c.db.QueryRowContext(ctx, query, id).Scan(
		&roa.ID, &roa.ASNID, &roa.PrefixID, &roa.MaxLength,
		&roa.NotBefore, &roa.NotAfter, &roa.Signature, &roa.TALID,
		&roa.CreatedAt, &roa.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ROA by ID: %w", err)
	}

	return &roa, nil
}

// GetROAs retrieves ROAs with pagination and filtering
func (c *PostgresClient) GetROAs(ctx context.Context, first, offset int, orderBy, filter interface{}) ([]*model.ROA, error) {
	query := `
		SELECT id, asn_id, prefix_id, max_length, not_before, not_after, signature, tal_id, created_at, updated_at
		FROM roas
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 0

	// Add filter conditions if provided
	if filter != nil {
		if f, ok := filter.(map[string]interface{}); ok {
			if asnID, ok := f["asnId"].(string); ok && asnID != "" {
				argCount++
				query += fmt.Sprintf(" AND asn_id = $%d", argCount)
				args = append(args, asnID)
			}
			if prefixID, ok := f["prefixId"].(string); ok && prefixID != "" {
				argCount++
				query += fmt.Sprintf(" AND prefix_id = $%d", argCount)
				args = append(args, prefixID)
			}
			if talID, ok := f["talId"].(string); ok && talID != "" {
				argCount++
				query += fmt.Sprintf(" AND tal_id = $%d", argCount)
				args = append(args, talID)
			}
		}
	}

	// Add ordering
	if orderBy != nil {
		if ob, ok := orderBy.(map[string]interface{}); ok {
			if field, ok := ob["field"].(string); ok {
				if direction, ok := ob["direction"].(string); ok {
					// Validate field and direction to prevent SQL injection
					validFields := map[string]bool{"asn_id": true, "prefix_id": true, "max_length": true, "not_before": true, "not_after": true}
					validDirections := map[string]bool{"ASC": true, "DESC": true}

					if validFields[field] && validDirections[direction] {
						query += fmt.Sprintf(" ORDER BY %s %s", field, direction)
					}
				}
			}
		}
	}

	// Add pagination
	if first > 0 {
		argCount++
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, first)
	}

	if offset > 0 {
		argCount++
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, offset)
	}

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query ROAs: %w", err)
	}
	defer rows.Close()

	var roas []*model.ROA
	for rows.Next() {
		var roa model.ROA
		err := rows.Scan(
			&roa.ID, &roa.ASNID, &roa.PrefixID, &roa.MaxLength,
			&roa.NotBefore, &roa.NotAfter, &roa.Signature, &roa.TALID,
			&roa.CreatedAt, &roa.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ROA row: %w", err)
		}
		roas = append(roas, &roa)
	}

	return roas, rows.Err()
}

// GetVRPByID retrieves a VRP by its ID
func (c *PostgresClient) GetVRPByID(ctx context.Context, id string) (*model.VRP, error) {
	query := `
		SELECT id, asn_id, prefix_id, max_length, not_before, not_after, roa_id, created_at, updated_at
		FROM vrps
		WHERE id = $1
	`

	var vrp model.VRP
	err := c.db.QueryRowContext(ctx, query, id).Scan(
		&vrp.ID, &vrp.ASNID, &vrp.PrefixID, &vrp.MaxLength,
		&vrp.NotBefore, &vrp.NotAfter, &vrp.ROAID,
		&vrp.CreatedAt, &vrp.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get VRP by ID: %w", err)
	}

	return &vrp, nil
}

// GetVRPs retrieves VRPs with pagination and filtering
func (c *PostgresClient) GetVRPs(ctx context.Context, first, offset int, orderBy, filter interface{}) ([]*model.VRP, error) {
	query := `
		SELECT id, asn_id, prefix_id, max_length, not_before, not_after, roa_id, created_at, updated_at
		FROM vrps
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 0

	// Add filter conditions if provided
	if filter != nil {
		if f, ok := filter.(map[string]interface{}); ok {
			if asnID, ok := f["asnId"].(string); ok && asnID != "" {
				argCount++
				query += fmt.Sprintf(" AND asn_id = $%d", argCount)
				args = append(args, asnID)
			}
			if prefixID, ok := f["prefixId"].(string); ok && prefixID != "" {
				argCount++
				query += fmt.Sprintf(" AND prefix_id = $%d", argCount)
				args = append(args, prefixID)
			}
			if roaID, ok := f["roaId"].(string); ok && roaID != "" {
				argCount++
				query += fmt.Sprintf(" AND roa_id = $%d", argCount)
				args = append(args, roaID)
			}
		}
	}

	// Add ordering
	if orderBy != nil {
		if ob, ok := orderBy.(map[string]interface{}); ok {
			if field, ok := ob["field"].(string); ok {
				if direction, ok := ob["direction"].(string); ok {
					// Validate field and direction to prevent SQL injection
					validFields := map[string]bool{"asn_id": true, "prefix_id": true, "max_length": true, "not_before": true, "not_after": true}
					validDirections := map[string]bool{"ASC": true, "DESC": true}

					if validFields[field] && validDirections[direction] {
						query += fmt.Sprintf(" ORDER BY %s %s", field, direction)
					}
				}
			}
		}
	}

	// Add pagination
	if first > 0 {
		argCount++
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, first)
	}

	if offset > 0 {
		argCount++
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, offset)
	}

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query VRPs: %w", err)
	}
	defer rows.Close()

	var vrps []*model.VRP
	for rows.Next() {
		var vrp model.VRP
		err := rows.Scan(
			&vrp.ID, &vrp.ASNID, &vrp.PrefixID, &vrp.MaxLength,
			&vrp.NotBefore, &vrp.NotAfter, &vrp.ROAID,
			&vrp.CreatedAt, &vrp.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan VRP row: %w", err)
		}
		vrps = append(vrps, &vrp)
	}

	return vrps, rows.Err()
}

// GetTrustAnchors retrieves all trust anchors
func (c *PostgresClient) GetTrustAnchors(ctx context.Context) ([]*model.TrustAnchor, error) {
	query := `
		SELECT id, name, uri, rsa_key, sha256, created_at, updated_at
		FROM trust_anchors
		ORDER BY name
	`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query trust anchors: %w", err)
	}
	defer rows.Close()

	var trustAnchors []*model.TrustAnchor
	for rows.Next() {
		var ta model.TrustAnchor
		err := rows.Scan(
			&ta.ID, &ta.Name, &ta.URI, &ta.RSAKey, &ta.SHA256,
			&ta.CreatedAt, &ta.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trust anchor row: %w", err)
		}
		trustAnchors = append(trustAnchors, &ta)
	}

	return trustAnchors, rows.Err()
}

// GetGlobalSummary calculates global summary statistics
func (c *PostgresClient) GetGlobalSummary(ctx context.Context) (*model.GlobalSummary, error) {
	// Get basic counts
	var summary model.GlobalSummary

	// Count ASNs
	err := c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM asns").Scan(&summary.TotalASNs)
	if err != nil {
		return nil, fmt.Errorf("failed to count ASNs: %w", err)
	}

	// Count prefixes
	err = c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM prefixes").Scan(&summary.TotalPrefixes)
	if err != nil {
		return nil, fmt.Errorf("failed to count prefixes: %w", err)
	}

	// Count ROAs
	err = c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM roas").Scan(&summary.TotalROAs)
	if err != nil {
		return nil, fmt.Errorf("failed to count ROAs: %w", err)
	}

	// Count VRPs
	err = c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM vrps").Scan(&summary.TotalVRPs)
	if err != nil {
		return nil, fmt.Errorf("failed to count VRPs: %w", err)
	}

	// Count prefixes by validation state
	err = c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM prefixes WHERE validation_state = 'VALID'").Scan(&summary.ValidPrefixes)
	if err != nil {
		return nil, fmt.Errorf("failed to count valid prefixes: %w", err)
	}

	err = c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM prefixes WHERE validation_state = 'INVALID'").Scan(&summary.InvalidPrefixes)
	if err != nil {
		return nil, fmt.Errorf("failed to count invalid prefixes: %w", err)
	}

	err = c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM prefixes WHERE validation_state = 'NOT_FOUND'").Scan(&summary.NotFoundPrefixes)
	if err != nil {
		return nil, fmt.Errorf("failed to count not found prefixes: %w", err)
	}

	// Get RIR stats
	rirQuery := `
		SELECT 
			CASE 
				WHEN ta.name LIKE '%%RIPE%%' THEN 'RIPE NCC'
				WHEN ta.name LIKE '%%APNIC%%' THEN 'APNIC'
				WHEN ta.name LIKE '%%ARIN%%' THEN 'ARIN'
				WHEN ta.name LIKE '%%LACNIC%%' THEN 'LACNIC'
				WHEN ta.name LIKE '%%AFRINIC%%' THEN 'AFRINIC'
				ELSE 'Other'
			END as rir,
			COUNT(DISTINCT r.id) as total_roas,
			COUNT(DISTINCT v.id) as total_vrps,
			COUNT(DISTINCT p.id) FILTER (WHERE p.validation_state = 'VALID') as valid_prefixes,
			COUNT(DISTINCT p.id) FILTER (WHERE p.validation_state = 'INVALID') as invalid_prefixes,
			COUNT(DISTINCT p.id) FILTER (WHERE p.validation_state = 'NOT_FOUND') as not_found_prefixes
		FROM trust_anchors ta
		LEFT JOIN roas r ON ta.id = r.tal_id
		LEFT JOIN vrps v ON r.id = v.roa_id
		LEFT JOIN prefixes p ON v.prefix_id = p.id
		GROUP BY rir
		ORDER BY rir
	`

	rows, err := c.db.QueryContext(ctx, rirQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query RIR stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var stat model.RIRStat
		err := rows.Scan(
			&stat.RIR, &stat.TotalROAs, &stat.TotalVRPs,
			&stat.ValidPrefixes, &stat.InvalidPrefixes, &stat.NotFoundPrefixes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan RIR stat row: %w", err)
		}
		summary.RIRStats = append(summary.RIRStats, stat)
	}

	return &summary, rows.Err()
}

// GetVRPsByASN retrieves VRPs for a specific ASN
func (c *PostgresClient) GetVRPsByASN(ctx context.Context, asnID string) ([]*model.VRP, error) {
	query := `
		SELECT id, asn_id, prefix_id, max_length, not_before, not_after, roa_id, created_at, updated_at
		FROM vrps
		WHERE asn_id = $1
		ORDER BY not_after DESC
	`

	rows, err := c.db.QueryContext(ctx, query, asnID)
	if err != nil {
		return nil, fmt.Errorf("failed to query VRPs by ASN: %w", err)
	}
	defer rows.Close()

	var vrps []*model.VRP
	for rows.Next() {
		var vrp model.VRP
		err := rows.Scan(
			&vrp.ID, &vrp.ASNID, &vrp.PrefixID, &vrp.MaxLength,
			&vrp.NotBefore, &vrp.NotAfter, &vrp.ROAID,
			&vrp.CreatedAt, &vrp.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan VRP row: %w", err)
		}
		vrps = append(vrps, &vrp)
	}

	return vrps, rows.Err()
}

// GetVRPsByPrefix retrieves VRPs for a specific prefix
func (c *PostgresClient) GetVRPsByPrefix(ctx context.Context, prefixID string) ([]*model.VRP, error) {
	query := `
		SELECT id, asn_id, prefix_id, max_length, not_before, not_after, roa_id, created_at, updated_at
		FROM vrps
		WHERE prefix_id = $1
		ORDER BY not_after DESC
	`

	rows, err := c.db.QueryContext(ctx, query, prefixID)
	if err != nil {
		return nil, fmt.Errorf("failed to query VRPs by prefix: %w", err)
	}
	defer rows.Close()

	var vrps []*model.VRP
	for rows.Next() {
		var vrp model.VRP
		err := rows.Scan(
			&vrp.ID, &vrp.ASNID, &vrp.PrefixID, &vrp.MaxLength,
			&vrp.NotBefore, &vrp.NotAfter, &vrp.ROAID,
			&vrp.CreatedAt, &vrp.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan VRP row: %w", err)
		}
		vrps = append(vrps, &vrp)
	}

	return vrps, rows.Err()
}

// GetROAsByASN retrieves ROAs for a specific ASN
func (c *PostgresClient) GetROAsByASN(ctx context.Context, asnID string) ([]*model.ROA, error) {
	query := `
		SELECT id, asn_id, prefix_id, max_length, not_before, not_after, signature, tal_id, created_at, updated_at
		FROM roas
		WHERE asn_id = $1
		ORDER BY not_after DESC
	`

	rows, err := c.db.QueryContext(ctx, query, asnID)
	if err != nil {
		return nil, fmt.Errorf("failed to query ROAs by ASN: %w", err)
	}
	defer rows.Close()

	var roas []*model.ROA
	for rows.Next() {
		var roa model.ROA
		err := rows.Scan(
			&roa.ID, &roa.ASNID, &roa.PrefixID, &roa.MaxLength,
			&roa.NotBefore, &roa.NotAfter, &roa.Signature, &roa.TALID,
			&roa.CreatedAt, &roa.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ROA row: %w", err)
		}
		roas = append(roas, &roa)
	}

	return roas, rows.Err()
}

// GetROAsByPrefix retrieves ROAs for a specific prefix
func (c *PostgresClient) GetROAsByPrefix(ctx context.Context, prefixID string) ([]*model.ROA, error) {
	query := `
		SELECT id, asn_id, prefix_id, max_length, not_before, not_after, signature, tal_id, created_at, updated_at
		FROM roas
		WHERE prefix_id = $1
		ORDER BY not_after DESC
	`

	rows, err := c.db.QueryContext(ctx, query, prefixID)
	if err != nil {
		return nil, fmt.Errorf("failed to query ROAs by prefix: %w", err)
	}
	defer rows.Close()

	var roas []*model.ROA
	for rows.Next() {
		var roa model.ROA
		err := rows.Scan(
			&roa.ID, &roa.ASNID, &roa.PrefixID, &roa.MaxLength,
			&roa.NotBefore, &roa.NotAfter, &roa.Signature, &roa.TALID,
			&roa.CreatedAt, &roa.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ROA row: %w", err)
		}
		roas = append(roas, &roa)
	}

	return roas, rows.Err()
}

// GetCertificatesByTrustAnchor retrieves certificates for a trust anchor
func (c *PostgresClient) GetCertificatesByTrustAnchor(ctx context.Context, talID string) ([]*model.Certificate, error) {
	query := `
		SELECT c.id, c.subject, c.issuer, c.serial_number, c.not_before, c.not_after,
			   c.subject_key_identifier, c.authority_key_identifier, c.crl_distribution_points,
			   c.authority_info_access, c.created_at, c.updated_at
		FROM certificates c
		JOIN roa_certificates rc ON c.id = rc.certificate_id
		JOIN roas r ON rc.roa_id = r.id
		WHERE r.tal_id = $1
		ORDER BY c.not_after DESC
	`

	rows, err := c.db.QueryContext(ctx, query, talID)
	if err != nil {
		return nil, fmt.Errorf("failed to query certificates by trust anchor: %w", err)
	}
	defer rows.Close()

	var certificates []*model.Certificate
	for rows.Next() {
		var cert model.Certificate
		err := rows.Scan(
			&cert.ID, &cert.Subject, &cert.Issuer, &cert.SerialNumber,
			&cert.NotBefore, &cert.NotAfter, &cert.SubjectKeyIdentifier,
			&cert.AuthorityKeyIdentifier, &cert.CRLDistributionPoints,
			&cert.AuthorityInfoAccess, &cert.CreatedAt, &cert.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan certificate row: %w", err)
		}
		certificates = append(certificates, &cert)
	}

	return certificates, rows.Err()
}

// GetCertificatesByROA retrieves certificates for a ROA
func (c *PostgresClient) GetCertificatesByROA(ctx context.Context, roaID string) ([]*model.Certificate, error) {
	query := `
		SELECT c.id, c.subject, c.issuer, c.serial_number, c.not_before, c.not_after,
			   c.subject_key_identifier, c.authority_key_identifier, c.crl_distribution_points,
			   c.authority_info_access, c.created_at, c.updated_at
		FROM certificates c
		JOIN roa_certificates rc ON c.id = rc.certificate_id
		WHERE rc.roa_id = $1
		ORDER BY c.not_after DESC
	`

	rows, err := c.db.QueryContext(ctx, query, roaID)
	if err != nil {
		return nil, fmt.Errorf("failed to query certificates by ROA: %w", err)
	}
	defer rows.Close()

	var certificates []*model.Certificate
	for rows.Next() {
		var cert model.Certificate
		err := rows.Scan(
			&cert.ID, &cert.Subject, &cert.Issuer, &cert.SerialNumber,
			&cert.NotBefore, &cert.NotAfter, &cert.SubjectKeyIdentifier,
			&cert.AuthorityKeyIdentifier, &cert.CRLDistributionPoints,
			&cert.AuthorityInfoAccess, &cert.CreatedAt, &cert.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan certificate row: %w", err)
		}
		certificates = append(certificates, &cert)
	}

	return certificates, rows.Err()
}

// GetTrustAnchorByID retrieves a trust anchor by its ID
func (c *PostgresClient) GetTrustAnchorByID(ctx context.Context, id string) (*model.TrustAnchor, error) {
	query := `
		SELECT id, name, uri, rsa_key, sha256, created_at, updated_at
		FROM trust_anchors
		WHERE id = $1
	`

	var ta model.TrustAnchor
	err := c.db.QueryRowContext(ctx, query, id).Scan(
		&ta.ID, &ta.Name, &ta.URI, &ta.RSAKey, &ta.SHA256,
		&ta.CreatedAt, &ta.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get trust anchor by ID: %w", err)
	}

	return &ta, nil
}

// GetTrustAnchorByName retrieves a trust anchor by its name
func (c *PostgresClient) GetTrustAnchorByName(ctx context.Context, name string) (*model.TrustAnchor, error) {
	query := `
		SELECT id, name, uri, rsa_key, sha256, created_at, updated_at
		FROM trust_anchors
		WHERE name = $1
	`

	var ta model.TrustAnchor
	err := c.db.QueryRowContext(ctx, query, name).Scan(
		&ta.ID, &ta.Name, &ta.URI, &ta.RSAKey, &ta.SHA256,
		&ta.CreatedAt, &ta.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get trust anchor by name: %w", err)
	}

	return &ta, nil
}

// InsertTrustAnchor inserts a new trust anchor into the database
func (c *PostgresClient) InsertTrustAnchor(ctx context.Context, ta *model.TrustAnchor) error {
	query := `
		INSERT INTO trust_anchors (id, name, uri, rsa_key, sha256, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := c.db.ExecContext(ctx, query,
		ta.ID, ta.Name, ta.URI, ta.RSAKey, ta.SHA256, ta.CreatedAt, ta.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert trust anchor: %w", err)
	}

	return nil
}

// GetOrCreateTrustAnchor gets an existing trust anchor by name or creates a new one
func (c *PostgresClient) GetOrCreateTrustAnchor(ctx context.Context, name, uri, rsaKey, sha256 string) (*model.TrustAnchor, error) {
	// First try to get existing trust anchor
	existing, err := c.GetTrustAnchorByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing trust anchor: %w", err)
	}

	if existing != nil {
		return existing, nil
	}

	// Create new trust anchor
	ta := &model.TrustAnchor{
		ID:        uuid.New().String(),
		Name:      name,
		URI:       uri,
		RSAKey:    rsaKey,
		SHA256:    sha256,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = c.InsertTrustAnchor(ctx, ta)
	if err != nil {
		return nil, fmt.Errorf("failed to insert new trust anchor: %w", err)
	}

	return ta, nil
}

// GetTrustAnchorByROA retrieves the trust anchor for a ROA
func (c *PostgresClient) GetTrustAnchorByROA(ctx context.Context, roaID string) (*model.TrustAnchor, error) {
	query := `
		SELECT ta.id, ta.name, ta.uri, ta.rsa_key, ta.sha256, ta.created_at, ta.updated_at
		FROM trust_anchors ta
		JOIN roas r ON ta.id = r.tal_id
		WHERE r.id = $1
	`

	var ta model.TrustAnchor
	err := c.db.QueryRowContext(ctx, query, roaID).Scan(
		&ta.ID, &ta.Name, &ta.URI, &ta.RSAKey, &ta.SHA256,
		&ta.CreatedAt, &ta.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get trust anchor by ROA: %w", err)
	}

	return &ta, nil
}

// GetASNByID retrieves an ASN by its ID
func (c *PostgresClient) GetASNByID(ctx context.Context, id string) (*model.ASN, error) {
	query := `
		SELECT id, number, name, country, created_at, updated_at
		FROM asns
		WHERE id = $1
	`

	var asn model.ASN
	err := c.db.QueryRowContext(ctx, query, id).Scan(
		&asn.ID, &asn.Number, &asn.Name, &asn.Country, &asn.CreatedAt, &asn.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ASN by ID: %w", err)
	}

	return &asn, nil
}

// GetPrefixByID retrieves a prefix by its ID
func (c *PostgresClient) GetPrefixByID(ctx context.Context, id string) (*model.Prefix, error) {
	query := `
		SELECT id, cidr, asn_id, max_length, expires_at, validation_state, created_at, updated_at
		FROM prefixes
		WHERE id = $1
	`

	var prefix model.Prefix
	err := c.db.QueryRowContext(ctx, query, id).Scan(
		&prefix.ID, &prefix.CIDR, &prefix.ASNID, &prefix.MaxLength,
		&prefix.ExpiresAt, &prefix.ValidationState, &prefix.CreatedAt, &prefix.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get prefix by ID: %w", err)
	}

	return &prefix, nil
}

// GetROAByVRP retrieves the ROA for a VRP
func (c *PostgresClient) GetROAByVRP(ctx context.Context, vrpID string) (*model.ROA, error) {
	query := `
		SELECT r.id, r.asn_id, r.prefix_id, r.max_length, r.not_before, r.not_after, r.signature, r.tal_id, r.created_at, r.updated_at
		FROM roas r
		JOIN vrps v ON r.id = v.roa_id
		WHERE v.id = $1
	`

	var roa model.ROA
	err := c.db.QueryRowContext(ctx, query, vrpID).Scan(
		&roa.ID, &roa.ASNID, &roa.PrefixID, &roa.MaxLength,
		&roa.NotBefore, &roa.NotAfter, &roa.Signature, &roa.TALID,
		&roa.CreatedAt, &roa.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ROA by VRP: %w", err)
	}

	return &roa, nil
}

// InsertASN inserts a new ASN into the database
func (c *PostgresClient) InsertASN(ctx context.Context, asn *model.ASN) error {
	query := `
		INSERT INTO asns (id, number, name, country, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (number) DO NOTHING
	`

	_, err := c.db.ExecContext(ctx, query,
		asn.ID, asn.Number, asn.Name, asn.Country, asn.CreatedAt, asn.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert ASN: %w", err)
	}

	return nil
}

// GetOrCreateASN gets an existing ASN by number or creates a new one
func (c *PostgresClient) GetOrCreateASN(ctx context.Context, number int, name, country string) (*model.ASN, error) {
	// First try to get existing ASN
	existing, err := c.GetASNByNumber(ctx, number)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing ASN: %w", err)
	}

	if existing != nil {
		return existing, nil
	}

	// Create new ASN
	asn := &model.ASN{
		ID:        uuid.New().String(),
		Number:    number,
		Name:      name,
		Country:   country,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = c.InsertASN(ctx, asn)
	if err != nil {
		return nil, fmt.Errorf("failed to insert new ASN: %w", err)
	}

	return asn, nil
}

// InsertPrefix inserts a new prefix into the database
func (c *PostgresClient) InsertPrefix(ctx context.Context, prefix *model.Prefix) error {
	query := `
		INSERT INTO prefixes (id, cidr, asn_id, max_length, expires_at, validation_state, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (cidr) DO NOTHING
	`

	_, err := c.db.ExecContext(ctx, query,
		prefix.ID, prefix.CIDR, prefix.ASNID, prefix.MaxLength,
		prefix.ExpiresAt, prefix.ValidationState, prefix.CreatedAt, prefix.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert prefix: %w", err)
	}

	return nil
}

// InsertROA inserts a new ROA into the database
func (c *PostgresClient) InsertROA(ctx context.Context, roa *model.ROA) error {
	query := `
		INSERT INTO roas (id, asn_id, prefix_id, max_length, not_before, not_after, signature, tal_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := c.db.ExecContext(ctx, query,
		roa.ID, roa.ASNID, roa.PrefixID, roa.MaxLength,
		roa.NotBefore, roa.NotAfter, roa.Signature, roa.TALID,
		roa.CreatedAt, roa.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert ROA: %w", err)
	}

	return nil
}

// InsertVRP inserts a new VRP into the database
func (c *PostgresClient) InsertVRP(ctx context.Context, vrp *model.VRP) error {
	query := `
		INSERT INTO vrps (id, asn_id, prefix_id, max_length, not_before, not_after, roa_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := c.db.ExecContext(ctx, query,
		vrp.ID, vrp.ASNID, vrp.PrefixID, vrp.MaxLength,
		vrp.NotBefore, vrp.NotAfter, vrp.ROAID,
		vrp.CreatedAt, vrp.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert VRP: %w", err)
	}

	return nil
}
