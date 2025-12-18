#!/usr/bin/env python3
"""
VRP Loader - Fetches and validates VRPs from Routinator
"""

import logging
import requests
from typing import Dict, List, Optional
from datetime import datetime
from pydantic import BaseModel, ValidationError, field_validator
import ipaddress

logger = logging.getLogger(__name__)

class VRPMetadata(BaseModel):
    """Metadata from Routinator JSON"""
    generated: int  # Unix timestamp
    
    @field_validator('generated')
    @classmethod
    def validate_timestamp(cls, v):
        if v <= 0:
            raise ValueError("Invalid timestamp")
        return v

class VRPEntry(BaseModel):
    """Single VRP entry with strict validation"""
    asn: str
    prefix: str
    maxLength: int
    ta: str
    
    @field_validator('asn')
    @classmethod
    def validate_asn(cls, v):
        if not v.startswith('AS'):
            raise ValueError("ASN must start with 'AS'")
        try:
            asn_num = int(v[2:])
            if asn_num < 0 or asn_num > 4294967295:
                raise ValueError("Invalid ASN range")
        except ValueError:
            raise ValueError("Invalid ASN format")
        return v
    
    @field_validator('prefix')
    @classmethod
    def validate_prefix(cls, v):
        try:
            # Validate CIDR notation
            ipaddress.ip_network(v, strict=True)
        except ValueError as e:
            raise ValueError(f"Invalid prefix: {e}")
        return v
    
    @field_validator('maxLength')
    @classmethod
    def validate_max_length(cls, v):
        if v < 0 or v > 128:
            raise ValueError("maxLength must be 0-128")
        return v
    
    def canonicalize(self) -> str:
        """Return canonical representation for deduplication"""
        # Normalize prefix
        network = ipaddress.ip_network(self.prefix, strict=True)
        normalized_prefix = str(network)
        
        return f"{self.asn}|{normalized_prefix}|{self.maxLength}|{self.ta}"

class RoutinagorVRPResponse(BaseModel):
    """Complete Routinator JSON response"""
    metadata: VRPMetadata
    roas: List[VRPEntry]

class VRPLoader:
    """Loads and validates VRPs from Routinator"""
    
    def __init__(self, routinator_url: str, diff_engine, metrics):
        self.routinator_url = routinator_url
        self.diff_engine = diff_engine
        self.metrics = metrics
        self.session = requests.Session()
        self.session.headers.update({
            'User-Agent': 'RPKI-Backend/1.0',
            'Accept': 'application/json'
        })
    
    def fetch_vrps(self) -> Optional[RoutinagorVRPResponse]:
        """
        Fetch VRPs from Routinator with strict validation
        
        Returns:
            Validated VRP response or None on failure
        """
        endpoint = f"{self.routinator_url}/json"
        
        try:
            logger.info(f"Fetching VRPs from {endpoint}")
            
            response = self.session.get(
                endpoint,
                timeout=60
            )
            
            # Check HTTP status
            if response.status_code != 200:
                logger.error(f"HTTP {response.status_code} from Routinator")
                self.metrics.fetch_failures.inc()
                return None
            
            # Parse and validate JSON
            try:
                data = response.json()
            except requests.exceptions.JSONDecodeError as e:
                logger.error(f"Invalid JSON from Routinator: {e}")
                self.metrics.fetch_failures.inc()
                return None
            
            # Validate schema with Pydantic
            try:
                vrp_response = RoutinagorVRPResponse(**data)
            except ValidationError as e:
                logger.error(f"Schema validation failed: {e}")
                self.metrics.fetch_failures.inc()
                return None
            
            # Success
            vrp_count = len(vrp_response.roas)
            logger.info(f"Successfully fetched {vrp_count} VRPs")
            self.metrics.vrp_count.set(vrp_count)
            self.metrics.last_successful_fetch.set_to_current_time()
            
            return vrp_response
            
        except requests.exceptions.Timeout:
            logger.error("Timeout fetching from Routinator")
            self.metrics.fetch_failures.inc()
            return None
            
        except requests.exceptions.ConnectionError as e:
            logger.error(f"Connection error: {e}")
            self.metrics.fetch_failures.inc()
            return None
            
        except Exception as e:
            logger.error(f"Unexpected error fetching VRPs: {e}", exc_info=True)
            self.metrics.fetch_failures.inc()
            return None
    
    def canonicalize_vrps(self, vrps: List[VRPEntry]) -> List[Dict]:
        """
        Canonicalize, deduplicate, and sort VRPs
        
        Returns:
            List of canonical VRP dictionaries
        """
        # Use dict to deduplicate by canonical form
        canonical_map = {}
        
        for vrp in vrps:
            canonical_key = vrp.canonicalize()
            
            if canonical_key not in canonical_map:
                # Store as dict for JSON serialization
                canonical_map[canonical_key] = {
                    'asn': vrp.asn,
                    'prefix': str(ipaddress.ip_network(vrp.prefix, strict=True)),
                    'maxLength': vrp.maxLength,
                    'ta': vrp.ta
                }
        
        # Sort deterministically for consistent hashing
        canonical_list = sorted(
            canonical_map.values(),
            key=lambda x: (x['prefix'], x['asn'], x['maxLength'])
        )
        
        original_count = len(vrps)
        deduplicated_count = len(canonical_list)
        
        if original_count != deduplicated_count:
            logger.info(
                f"Deduplicated {original_count} -> {deduplicated_count} VRPs"
            )
        
        return canonical_list
    
    def fetch_and_process(self) -> bool:
        """
        Complete fetch and processing workflow
        
        Returns:
            True if successful, False otherwise
        """
        # Fetch from Routinator
        vrp_response = self.fetch_vrps()
        
        if vrp_response is None:
            logger.warning("Keeping last snapshot due to fetch failure")
            return False
        
        # Canonicalize VRPs
        canonical_vrps = self.canonicalize_vrps(vrp_response.roas)
        
        # Pass to diff engine
        metadata = {
            'generated': vrp_response.metadata.generated,
            'fetched_at': datetime.utcnow().isoformat()
        }
        
        success = self.diff_engine.process_snapshot(canonical_vrps, metadata)
        
        if success:
            logger.info("VRP snapshot processed successfully")
        else:
            logger.warning("VRP snapshot rejected (no change)")
        
        return success