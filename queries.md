# Docker Commands for Testing RPKI Database Data

This document contains Docker commands to test and inspect the RPKI database data. All commands assume you're in the project root directory and the containers are running.

## Docker Commands for Testing Database Data

These commands can be used to test and inspect the database data using Docker Compose. All commands assume you're in the project root directory and the containers are running.

### Accessing the Database
```bash
# Enter PostgreSQL container
docker-compose exec db psql -U rpki_user -d rpki_viz

# Or run individual queries
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT COUNT(*) FROM asns;"
```

### Count Records in Each Table
```bash
# Count ASNs
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT COUNT(*) FROM asns;"

# Count prefixes
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT COUNT(*) FROM prefixes;"

# Count ROAs
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT COUNT(*) FROM roas;"

# Count VRPs
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT COUNT(*) FROM vrps;"

# Count trust anchors
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT COUNT(*) FROM trust_anchors;"

# Count certificates
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT COUNT(*) FROM certificates;"
```

### Validation State Counts
```bash
# Count prefixes by validation state
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT validation_state, COUNT(*) FROM prefixes GROUP BY validation_state;"

# Count valid prefixes
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT COUNT(*) FROM prefixes WHERE validation_state = 'VALID';"

# Count invalid prefixes
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT COUNT(*) FROM prefixes WHERE validation_state = 'INVALID';"

# Count not found prefixes
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT COUNT(*) FROM prefixes WHERE validation_state = 'NOT_FOUND';"
```

### Sample Data Queries
```bash
# View first 10 ASNs with all columns
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT * FROM asns LIMIT 10;"

# View first 10 prefixes with all columns
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT * FROM prefixes LIMIT 10;"

# View first 10 ROAs with all columns
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT * FROM roas LIMIT 10;"

# View first 10 VRPs with all columns
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT * FROM vrps LIMIT 10;"

# View all trust anchors
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT * FROM trust_anchors;"

# View first 10 certificates
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT * FROM certificates LIMIT 10;"

# View specific columns for readability
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT id, number, name, country FROM asns LIMIT 5;"

# View prefixes with validation state
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT id, cidr, asn_id, validation_state FROM prefixes LIMIT 5;"

# View trust anchors (name only for brevity)
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT id, name FROM trust_anchors;"

# View recent ROAs (last 5)
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT id, asn_id, prefix_id, tal_id FROM roas ORDER BY created_at DESC LIMIT 5;"

# View recent VRPs (last 5)
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT id, asn_id, prefix_id, roa_id FROM vrps ORDER BY created_at DESC LIMIT 5;"
```

### View Actual Data by Validation State
```bash
# View VALID prefixes
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT * FROM prefixes WHERE validation_state = 'VALID' LIMIT 10;"

# View INVALID prefixes
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT * FROM prefixes WHERE validation_state = 'INVALID' LIMIT 10;"

# View NOT_FOUND prefixes
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT * FROM prefixes WHERE validation_state = 'NOT_FOUND' LIMIT 10;"
```

### View Related Data (Joined Queries)
```bash
# View ASNs with their prefixes
docker-compose exec db psql -U rpki_user -d rpki_viz -c "
SELECT a.number, a.name, p.cidr, p.validation_state
FROM asns a
JOIN prefixes p ON a.id = p.asn_id
LIMIT 10;
"

# View ROAs with their trust anchors
docker-compose exec db psql -U rpki_user -d rpki_viz -c "
SELECT r.id, ta.name as trust_anchor, r.asn_id, r.prefix_id
FROM roas r
JOIN trust_anchors ta ON r.tal_id = ta.id
LIMIT 10;
"

# View VRPs with their ROAs and prefixes
docker-compose exec db psql -U rpki_user -d rpki_viz -c "
SELECT v.id, r.asn_id, p.cidr, v.max_length, v.not_before, v.not_after
FROM vrps v
JOIN roas r ON v.roa_id = r.id
JOIN prefixes p ON v.prefix_id = p.id
LIMIT 10;
"
```

### RIR Statistics Query
```bash
# Get RIR breakdown statistics
docker-compose exec db psql -U rpki_user -d rpki_viz -c "
SELECT
    CASE
        WHEN ta.name LIKE '%RIPE%' THEN 'RIPE NCC'
        WHEN ta.name LIKE '%APNIC%' THEN 'APNIC'
        WHEN ta.name LIKE '%ARIN%' THEN 'ARIN'
        WHEN ta.name LIKE '%LACNIC%' THEN 'LACNIC'
        WHEN ta.name LIKE '%AFRINIC%' THEN 'AFRINIC'
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
ORDER BY rir;
"
```

### Database Schema Inspection
```bash
# List all tables
docker-compose exec db psql -U rpki_user -d rpki_viz -c "\dt"

# Describe table structure (example for asns table)
docker-compose exec db psql -U rpki_user -d rpki_viz -c "\d asns"

# Show all indexes
docker-compose exec db psql -U rpki_user -d rpki_viz -c "SELECT tablename, indexname FROM pg_indexes WHERE schemaname = 'public';"
```