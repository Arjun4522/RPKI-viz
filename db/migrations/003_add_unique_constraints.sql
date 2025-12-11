-- Add unique constraints for ON CONFLICT clauses
ALTER TABLE roas ADD CONSTRAINT unique_roa UNIQUE (asn_id, prefix_id, max_length, tal_id);
ALTER TABLE vrps ADD CONSTRAINT unique_vrp UNIQUE (asn_id, prefix_id, max_length);