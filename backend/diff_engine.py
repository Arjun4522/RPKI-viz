#!/usr/bin/env python3
"""
Diff & Serial Engine - Maintains monotonic serial and computes VRP diffs
"""

import os
import json
import hashlib
import logging
from typing import Dict, List, Optional, Tuple
from datetime import datetime
from threading import RLock

logger = logging.getLogger(__name__)

class DiffEngine:
    """
    Manages VRP snapshots, diffs, and monotonic serials
    
    State persistence:
    - current_snapshot.json: Active VRP set
    - previous_snapshot.json: Previous VRP set
    - state_metadata.json: Serial and timestamps
    - diffs/: Historical diffs
    """
    
    def __init__(self, state_dir: str, metrics):
        self.state_dir = state_dir
        self.metrics = metrics
        
        # Thread-safe access to state
        self.lock = RLock()
        
        # In-memory state
        self.current_snapshot: List[Dict] = []
        self.previous_snapshot: List[Dict] = []
        self.serial: int = 0
        self.current_hash: str = ""
        self.last_update: Optional[str] = None
        
        # Paths
        self.current_snapshot_path = os.path.join(state_dir, 'current_snapshot.json')
        self.previous_snapshot_path = os.path.join(state_dir, 'previous_snapshot.json')
        self.metadata_path = os.path.join(state_dir, 'state_metadata.json')
        self.diffs_dir = os.path.join(state_dir, 'diffs')
        
        os.makedirs(self.diffs_dir, exist_ok=True)
    
    def _compute_hash(self, vrps: List[Dict]) -> str:
        """
        Compute deterministic hash of VRP snapshot
        
        Args:
            vrps: Canonicalized and sorted VRP list
        
        Returns:
            SHA256 hash as hex string
        """
        # Convert to JSON with sorted keys for determinism
        json_bytes = json.dumps(vrps, sort_keys=True).encode('utf-8')
        return hashlib.sha256(json_bytes).hexdigest()
    
    def _compute_diff(
        self,
        old_vrps: List[Dict],
        new_vrps: List[Dict]
    ) -> Tuple[List[Dict], List[Dict]]:
        """
        Compute added and removed VRPs
        
        Returns:
            (added, removed) tuple
        """
        # Convert to sets using canonical form for comparison
        def to_key(vrp):
            return f"{vrp['asn']}|{vrp['prefix']}|{vrp['maxLength']}"
        
        old_set = {to_key(v): v for v in old_vrps}
        new_set = {to_key(v): v for v in new_vrps}
        
        added = [new_set[k] for k in (new_set.keys() - old_set.keys())]
        removed = [old_set[k] for k in (old_set.keys() - new_set.keys())]
        
        return added, removed
    
    def _save_diff(self, serial: int, added: List[Dict], removed: List[Dict], metadata: Dict):
        """Save diff to disk"""
        diff_path = os.path.join(self.diffs_dir, f'diff_{serial:010d}.json')
        
        diff_data = {
            'serial': serial,
            'timestamp': datetime.utcnow().isoformat(),
            'metadata': metadata,
            'added_count': len(added),
            'removed_count': len(removed),
            'added': added,
            'removed': removed
        }
        
        with open(diff_path, 'w') as f:
            json.dump(diff_data, f, indent=2)
        
        logger.info(f"Saved diff to {diff_path}")
    
    def _save_state(self):
        """Persist current state to disk"""
        # Save current snapshot
        with open(self.current_snapshot_path, 'w') as f:
            json.dump(self.current_snapshot, f, indent=2)
        
        # Save previous snapshot
        with open(self.previous_snapshot_path, 'w') as f:
            json.dump(self.previous_snapshot, f, indent=2)
        
        # Save metadata
        metadata = {
            'serial': self.serial,
            'current_hash': self.current_hash,
            'last_update': self.last_update,
            'vrp_count': len(self.current_snapshot)
        }
        
        with open(self.metadata_path, 'w') as f:
            json.dump(metadata, f, indent=2)
        
        logger.info(f"Persisted state: serial={self.serial}, hash={self.current_hash[:8]}")
    
    def load_state(self):
        """Load state from disk on startup"""
        with self.lock:
            try:
                # Load metadata
                with open(self.metadata_path, 'r') as f:
                    metadata = json.load(f)
                
                self.serial = metadata['serial']
                self.current_hash = metadata['current_hash']
                self.last_update = metadata.get('last_update')
                
                # Load current snapshot
                with open(self.current_snapshot_path, 'r') as f:
                    self.current_snapshot = json.load(f)
                
                # Load previous snapshot
                try:
                    with open(self.previous_snapshot_path, 'r') as f:
                        self.previous_snapshot = json.load(f)
                except FileNotFoundError:
                    self.previous_snapshot = []
                
                logger.info(
                    f"Loaded state: serial={self.serial}, "
                    f"vrps={len(self.current_snapshot)}, "
                    f"hash={self.current_hash[:8]}"
                )
                
                # Update metrics
                self.metrics.serial_number.set(self.serial)
                self.metrics.vrp_count.set(len(self.current_snapshot))
                
            except FileNotFoundError:
                logger.info("No existing state found, starting fresh")
                self.serial = 0
                self.current_snapshot = []
                self.previous_snapshot = []
                self.current_hash = ""
                self.last_update = None
    
    def process_snapshot(
        self,
        vrps: List[Dict],
        metadata: Dict
    ) -> bool:
        """
        Process new VRP snapshot
        
        Args:
            vrps: Canonicalized VRP list
            metadata: Routinator metadata
        
        Returns:
            True if snapshot accepted (hash changed), False if rejected (no change)
        """
        with self.lock:
            # Compute hash of new snapshot
            new_hash = self._compute_hash(vrps)
            
            # Check if snapshot has changed
            if new_hash == self.current_hash:
                logger.info("Snapshot hash unchanged, no update needed")
                return False
            
            logger.info(
                f"Snapshot hash changed: {self.current_hash[:8]} -> {new_hash[:8]}"
            )
            
            # Compute diff
            added, removed = self._compute_diff(self.current_snapshot, vrps)
            
            logger.info(f"Diff: +{len(added)} -{len(removed)}")
            
            # Increment serial (monotonic)
            new_serial = self.serial + 1
            
            # Update state
            self.previous_snapshot = self.current_snapshot
            self.current_snapshot = vrps
            self.serial = new_serial
            self.current_hash = new_hash
            self.last_update = datetime.utcnow().isoformat()
            
            # Save diff
            self._save_diff(new_serial, added, removed, metadata)
            
            # Persist state
            self._save_state()
            
            # Update metrics
            self.metrics.serial_number.set(self.serial)
            self.metrics.vrp_count.set(len(self.current_snapshot))
            self.metrics.snapshot_age.set_function(self._get_snapshot_age)
            
            logger.info(f"Snapshot accepted: serial={self.serial}")
            
            return True
    
    def _get_snapshot_age(self):
        """Calculate snapshot age in seconds"""
        if self.last_update is None:
            return 0
        
        try:
            last_update_dt = datetime.fromisoformat(self.last_update)
            age = (datetime.utcnow() - last_update_dt).total_seconds()
            return age
        except Exception:
            return 0
    
    def get_current_state(self) -> Dict:
        """Get current state for API"""
        with self.lock:
            return {
                'serial': self.serial,
                'vrp_count': len(self.current_snapshot),
                'hash': self.current_hash,
                'last_update': self.last_update,
                'vrps': self.current_snapshot
            }
    
    def get_diff(self, from_serial: int, to_serial: int) -> Optional[Dict]:
        """
        Retrieve a specific diff
        
        Args:
            from_serial: Starting serial
            to_serial: Ending serial
        
        Returns:
            Diff data or None if not found
        """
        if to_serial < from_serial:
            return None
        
        # For now, only support single-step diffs
        if to_serial - from_serial != 1:
            logger.warning(f"Multi-step diff not supported: {from_serial} -> {to_serial}")
            return None
        
        diff_path = os.path.join(self.diffs_dir, f'diff_{to_serial:010d}.json')
        
        try:
            with open(diff_path, 'r') as f:
                return json.load(f)
        except FileNotFoundError:
            logger.warning(f"Diff not found: {diff_path}")
            return None