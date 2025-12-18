#!/usr/bin/env python3
"""
Production RPKI Backend
Integrates with Routinator for VRP validation
"""

import os
import sys
import time
import signal
import logging
from threading import Event

from vrp_loader import VRPLoader
from diff_engine import DiffEngine
from api_server import APIServer
from metrics import MetricsCollector

# Configure structured logging
logging.basicConfig(
    level=os.getenv('LOG_LEVEL', 'INFO'),
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)

logger = logging.getLogger(__name__)

class RPKIBackend:
    """Main application controller"""
    
    def __init__(self):
        self.shutdown_event = Event()
        self.routinator_url = os.getenv('ROUTINATOR_URL', 'http://routinator:8323')
        self.poll_interval = int(os.getenv('POLL_INTERVAL_SECONDS', '600'))
        self.state_dir = os.getenv('STATE_DIR', '/app/state')
        self.api_port = int(os.getenv('API_PORT', '8080'))
        
        # Ensure state directory exists
        os.makedirs(self.state_dir, exist_ok=True)
        
        # Initialize components
        self.metrics = MetricsCollector()
        self.diff_engine = DiffEngine(self.state_dir, self.metrics)
        self.vrp_loader = VRPLoader(
            self.routinator_url,
            self.diff_engine,
            self.metrics
        )
        self.api_server = APIServer(
            self.diff_engine,
            self.metrics,
            self.api_port
        )
        
        # Register signal handlers
        signal.signal(signal.SIGTERM, self._signal_handler)
        signal.signal(signal.SIGINT, self._signal_handler)
    
    def _signal_handler(self, signum, frame):
        """Handle shutdown signals gracefully"""
        logger.info(f"Received signal {signum}, initiating shutdown...")
        self.shutdown_event.set()
    
    def run(self):
        """Main application loop"""
        logger.info("Starting RPKI Backend")
        logger.info(f"Routinator URL: {self.routinator_url}")
        logger.info(f"Poll interval: {self.poll_interval}s")
        logger.info(f"State directory: {self.state_dir}")
        
        # Start API server in background
        self.api_server.start()
        
        # Load existing state on startup
        try:
            self.diff_engine.load_state()
            logger.info("Loaded existing state from disk")
        except Exception as e:
            logger.warning(f"No existing state found: {e}")
        
        # Initial fetch
        logger.info("Performing initial VRP fetch...")
        try:
            self.vrp_loader.fetch_and_process()
        except Exception as e:
            logger.error(f"Initial fetch failed: {e}")
            self.metrics.fetch_failures.inc()
        
        # Main polling loop
        last_fetch = time.time()
        
        while not self.shutdown_event.is_set():
            try:
                now = time.time()
                time_since_last = now - last_fetch
                
                if time_since_last >= self.poll_interval:
                    logger.info("Starting scheduled VRP fetch...")
                    self.vrp_loader.fetch_and_process()
                    last_fetch = now
                
                # Sleep in short intervals to allow quick shutdown
                self.shutdown_event.wait(timeout=10)
                
            except Exception as e:
                logger.error(f"Error in main loop: {e}", exc_info=True)
                self.metrics.fetch_failures.inc()
                self.shutdown_event.wait(timeout=60)
        
        # Shutdown
        logger.info("Shutting down RPKI Backend")
        self.api_server.stop()
        logger.info("Shutdown complete")

def main():
    """Entry point"""
    try:
        backend = RPKIBackend()
        backend.run()
    except Exception as e:
        logger.critical(f"Fatal error: {e}", exc_info=True)
        sys.exit(1)

if __name__ == '__main__':
    main()