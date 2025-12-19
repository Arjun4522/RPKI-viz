import React, { useState, useEffect } from 'react'
import { rpkiApi } from '../services/api'

function Dashboard() {
  const [state, setState] = useState(null)
  const [metrics, setMetrics] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  useEffect(() => {
    fetchDashboardData()
    const interval = setInterval(fetchDashboardData, 10000)
    return () => clearInterval(interval)
  }, [])

  const fetchDashboardData = async () => {
    try {
      setLoading(true)
      const [stateResponse, metricsResponse] = await Promise.all([
        rpkiApi.getState(),
        rpkiApi.getMetrics()
      ])
      
      setState(stateResponse.data)
      
      // Parse Prometheus metrics
      const metricsText = metricsResponse.data
      const metricsData = {}
      metricsText.split('\n').forEach(line => {
        if (line && !line.startsWith('#')) {
          const [metric, value] = line.split(' ')
          if (metric && value) {
            metricsData[metric] = parseFloat(value)
          }
        }
      })
      setMetrics(metricsData)
      setError(null)
    } catch (err) {
      setError('Failed to fetch dashboard data')
    } finally {
      setLoading(false)
    }
  }

  if (loading) return <div className="dashboard-loading">Loading dashboard...</div>
  if (error) return <div className="dashboard-error">{error}</div>

  return (
    <div className="dashboard">
      <div className="dashboard-grid">
        <div className="dashboard-card">
          <h3>Current State</h3>
          <div className="stat">
            <span className="stat-label">Serial:</span>
            <span className="stat-value">{state?.serial || 'N/A'}</span>
          </div>
          <div className="stat">
            <span className="stat-label">VRP Count:</span>
            <span className="stat-value">{state?.vrp_count?.toLocaleString() || '0'}</span>
          </div>
          <div className="stat">
            <span className="stat-label">Last Update:</span>
            <span className="stat-value">
              {state?.last_update ? new Date(state.last_update).toLocaleString() : 'Never'}
            </span>
          </div>
        </div>

        <div className="dashboard-card">
          <h3>System Metrics</h3>
          <div className="stat">
            <span className="stat-label">Fetch Failures:</span>
            <span className="stat-value">
              {metrics?.rpki_fetch_failures_total || 0}
            </span>
          </div>
          <div className="stat">
            <span className="stat-label">Snapshot Age:</span>
            <span className="stat-value">
              {metrics?.rpki_snapshot_age_seconds 
                ? `${Math.floor(metrics.rpki_snapshot_age_seconds / 60)}m ${Math.floor(metrics.rpki_snapshot_age_seconds % 60)}s`
                : 'N/A'
              }
            </span>
          </div>
          <div className="stat">
            <span className="stat-label">Last Fetch:</span>
            <span className="stat-value">
              {metrics?.rpki_last_successful_fetch_timestamp 
                ? new Date(metrics.rpki_last_successful_fetch_timestamp * 1000).toLocaleString()
                : 'Never'
              }
            </span>
          </div>
        </div>

        <div className="dashboard-card">
          <h3>Quick Actions</h3>
          <button 
            className="action-btn"
            onClick={fetchDashboardData}
          >
            Refresh Data
          </button>
          <button 
            className="action-btn"
            onClick={() => window.open('/api/v1/vrps', '_blank')}
          >
            View Raw VRPs
          </button>
          <button 
            className="action-btn"
            onClick={() => window.open('/metrics', '_blank')}
          >
            View Metrics
          </button>
        </div>
      </div>

      {state?.hash && (
        <div className="hash-info">
          <small>Current Hash: {state.hash.substring(0, 16)}...</small>
        </div>
      )}
    </div>
  )
}

export default Dashboard