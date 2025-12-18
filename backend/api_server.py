#!/usr/bin/env python3
"""
API Server - Exposes VRP data and observability endpoints
"""

import logging
from flask import Flask, jsonify, request
from threading import Thread
from werkzeug.serving import make_server
from datetime import datetime

logger = logging.getLogger(__name__)

class APIServer:
    """Flask-based API server"""
    
    def __init__(self, diff_engine, metrics, port):
        self.diff_engine = diff_engine
        self.metrics = metrics
        self.port = port
        
        # Create Flask app
        self.app = Flask(__name__)
        self.app.json.sort_keys = False
        
        # Disable Flask's default logger
        log = logging.getLogger('werkzeug')
        log.setLevel(logging.WARNING)
        
        # Register routes
        self._register_routes()
        
        # Server thread
        self.server = None
        self.server_thread = None
    
    def _register_routes(self):
        """Register all API endpoints"""
        
        @self.app.route('/health', methods=['GET'])
        def health():
            """Health check endpoint"""
            state = self.diff_engine.get_current_state()
            
            status = {
                'status': 'healthy',
                'serial': state['serial'],
                'vrp_count': state['vrp_count'],
                'last_update': state['last_update']
            }
            
            self.metrics.api_requests.labels(
                endpoint='/health',
                method='GET',
                status='200'
            ).inc()
            
            return jsonify(status), 200
        
        @self.app.route('/metrics', methods=['GET'])
        def metrics():
            """Prometheus metrics endpoint"""
            self.metrics.api_requests.labels(
                endpoint='/metrics',
                method='GET',
                status='200'
            ).inc()
            
            return self.metrics.get_metrics(), 200, {
                'Content-Type': self.metrics.get_content_type()
            }
        
        @self.app.route('/api/v1/state', methods=['GET'])
        def get_state():
            """Get current RPKI state"""
            with self.metrics.api_request_duration.labels(
                endpoint='/api/v1/state',
                method='GET'
            ).time():
                state = self.diff_engine.get_current_state()
                
                response = {
                    'serial': state['serial'],
                    'vrp_count': state['vrp_count'],
                    'hash': state['hash'],
                    'last_update': state['last_update']
                }
                
                self.metrics.api_requests.labels(
                    endpoint='/api/v1/state',
                    method='GET',
                    status='200'
                ).inc()
                
                return jsonify(response), 200
        
        @self.app.route('/api/v1/vrps', methods=['GET'])
        def get_vrps():
            """Get all VRPs"""
            with self.metrics.api_request_duration.labels(
                endpoint='/api/v1/vrps',
                method='GET'
            ).time():
                state = self.diff_engine.get_current_state()
                
                # Optional filtering
                asn_filter = request.args.get('asn')
                prefix_filter = request.args.get('prefix')
                
                vrps = state['vrps']
                
                if asn_filter:
                    vrps = [v for v in vrps if v['asn'] == asn_filter]
                
                if prefix_filter:
                    vrps = [v for v in vrps if v['prefix'] == prefix_filter]
                
                response = {
                    'serial': state['serial'],
                    'total_vrps': state['vrp_count'],
                    'filtered_vrps': len(vrps),
                    'last_update': state['last_update'],
                    'vrps': vrps
                }
                
                self.metrics.api_requests.labels(
                    endpoint='/api/v1/vrps',
                    method='GET',
                    status='200'
                ).inc()
                
                return jsonify(response), 200
        
        @self.app.route('/api/v1/diff', methods=['GET'])
        def get_diff():
            """Get diff between serials"""
            try:
                from_serial = int(request.args.get('from', 0))
                to_serial = int(request.args.get('to', 0))
            except ValueError:
                self.metrics.api_requests.labels(
                    endpoint='/api/v1/diff',
                    method='GET',
                    status='400'
                ).inc()
                return jsonify({'error': 'Invalid serial parameters'}), 400
            
            with self.metrics.api_request_duration.labels(
                endpoint='/api/v1/diff',
                method='GET'
            ).time():
                diff = self.diff_engine.get_diff(from_serial, to_serial)
                
                if diff is None:
                    self.metrics.api_requests.labels(
                        endpoint='/api/v1/diff',
                        method='GET',
                        status='404'
                    ).inc()
                    return jsonify({'error': 'Diff not found'}), 404
                
                self.metrics.api_requests.labels(
                    endpoint='/api/v1/diff',
                    method='GET',
                    status='200'
                ).inc()
                
                return jsonify(diff), 200
        
        @self.app.route('/api/v1/validate', methods=['POST'])
        def validate_route():
            """Validate a BGP route announcement"""
            data = request.get_json()
            
            if not data or 'asn' not in data or 'prefix' not in data:
                self.metrics.api_requests.labels(
                    endpoint='/api/v1/validate',
                    method='POST',
                    status='400'
                ).inc()
                return jsonify({'error': 'Missing asn or prefix'}), 400
            
            with self.metrics.api_request_duration.labels(
                endpoint='/api/v1/validate',
                method='POST'
            ).time():
                asn = data['asn']
                prefix = data['prefix']
                
                # Ensure ASN has AS prefix
                if not asn.startswith('AS'):
                    asn = f'AS{asn}'
                
                state = self.diff_engine.get_current_state()
                
                # Find matching VRP
                matching_vrps = []
                for vrp in state['vrps']:
                    if vrp['asn'] == asn and vrp['prefix'] == prefix:
                        matching_vrps.append(vrp)
                
                result = {
                    'asn': asn,
                    'prefix': prefix,
                    'valid': len(matching_vrps) > 0,
                    'matching_vrps': matching_vrps,
                    'serial': state['serial']
                }
                
                self.metrics.api_requests.labels(
                    endpoint='/api/v1/validate',
                    method='POST',
                    status='200'
                ).inc()
                
                return jsonify(result), 200
        
        @self.app.errorhandler(404)
        def not_found(e):
            self.metrics.api_requests.labels(
                endpoint='unknown',
                method=request.method,
                status='404'
            ).inc()
            return jsonify({'error': 'Not found'}), 404
        
        @self.app.errorhandler(500)
        def internal_error(e):
            logger.error(f"Internal error: {e}", exc_info=True)
            self.metrics.api_requests.labels(
                endpoint=request.path,
                method=request.method,
                status='500'
            ).inc()
            return jsonify({'error': 'Internal server error'}), 500
    
    def start(self):
        """Start API server in background thread"""
        self.server = make_server('0.0.0.0', self.port, self.app, threaded=True)
        self.server_thread = Thread(target=self.server.serve_forever, daemon=True)
        self.server_thread.start()
        logger.info(f"API server started on port {self.port}")
    
    def stop(self):
        """Stop API server"""
        if self.server:
            logger.info("Stopping API server...")
            self.server.shutdown()
            self.server_thread.join(timeout=5)
            logger.info("API server stopped")