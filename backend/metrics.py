#!/usr/bin/env python3
"""
Metrics Collection - Prometheus metrics for observability
"""

from prometheus_client import Counter, Gauge, Histogram, generate_latest, CONTENT_TYPE_LATEST

class MetricsCollector:
    """Centralized metrics collection"""
    
    def __init__(self):
        # Fetch metrics
        self.fetch_failures = Counter(
            'rpki_fetch_failures_total',
            'Total number of failed VRP fetches from Routinator'
        )
        
        self.last_successful_fetch = Gauge(
            'rpki_last_successful_fetch_timestamp',
            'Timestamp of last successful VRP fetch'
        )
        
        # VRP metrics
        self.vrp_count = Gauge(
            'rpki_vrp_count',
            'Current number of validated VRPs'
        )
        
        self.serial_number = Gauge(
            'rpki_serial_number',
            'Current monotonic serial number'
        )
        
        self.snapshot_age = Gauge(
            'rpki_snapshot_age_seconds',
            'Age of current VRP snapshot in seconds'
        )
        
        # Processing metrics
        self.snapshot_accepted = Counter(
            'rpki_snapshot_accepted_total',
            'Total number of accepted snapshots (hash changed)'
        )
        
        self.snapshot_rejected = Counter(
            'rpki_snapshot_rejected_total',
            'Total number of rejected snapshots (no hash change)'
        )
        
        # API metrics
        self.api_requests = Counter(
            'rpki_api_requests_total',
            'Total API requests',
            ['endpoint', 'method', 'status']
        )
        
        self.api_request_duration = Histogram(
            'rpki_api_request_duration_seconds',
            'API request duration in seconds',
            ['endpoint', 'method']
        )
    
    def get_metrics(self):
        """Return Prometheus metrics in text format"""
        return generate_latest()
    
    def get_content_type(self):
        """Return Prometheus content type"""
        return CONTENT_TYPE_LATEST