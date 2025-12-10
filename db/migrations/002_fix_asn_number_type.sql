-- Fix ASN number column type to support 64-bit integers
ALTER TABLE asns ALTER COLUMN number TYPE BIGINT;