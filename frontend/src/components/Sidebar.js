import React from 'react';
import './Sidebar.css';

function Sidebar({ activeView, onViewChange }) {
  const menuItems = [
    { id: 'global', label: 'Global Summary', icon: '📊' },
    { id: 'asns', label: 'ASN Statistics', icon: '🏢' },
    { id: 'prefixes', label: 'Prefix Statistics', icon: '🌐' },
    { id: 'roas', label: 'ROA Statistics', icon: '📋' },
    { id: 'vrps', label: 'VRP Statistics', icon: '✅' },
    { id: 'rir', label: 'RIR Statistics', icon: '🌍' },
    { id: 'validation', label: 'Validation Stats', icon: '🔍' },
  ];

  return (
    <div className="sidebar">
      <div className="sidebar-header">
        <h2>RPKI Dashboard</h2>
      </div>
      <nav className="sidebar-nav">
        {menuItems.map((item) => (
          <button
            key={item.id}
            className={`sidebar-item ${activeView === item.id ? 'active' : ''}`}
            onClick={() => onViewChange(item.id)}
          >
            <span className="sidebar-icon">{item.icon}</span>
            <span className="sidebar-label">{item.label}</span>
          </button>
        ))}
      </nav>
    </div>
  );
}

export default Sidebar;