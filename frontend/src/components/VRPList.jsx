import React, { useState, useEffect } from 'react'
import { rpkiApi } from '../services/api'

function VRPList() {
  const [vrps, setVrps] = useState([])
  const [filteredVrps, setFilteredVrps] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [filters, setFilters] = useState({
    asn: '',
    prefix: ''
  })
  const [page, setPage] = useState(1)
  const itemsPerPage = 50

  useEffect(() => {
    fetchVRPs()
  }, [])

  useEffect(() => {
    filterVRPs()
  }, [vrps, filters])

  const fetchVRPs = async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await rpkiApi.getVRPs(filters)
      setVrps(response.data.vrps || [])
    } catch (err) {
      setError('Failed to fetch VRPs')
      setVrps([])
    } finally {
      setLoading(false)
    }
  }

  const filterVRPs = () => {
    let filtered = vrps
    
    if (filters.asn) {
      filtered = filtered.filter(vrp => 
        vrp.asn.toLowerCase().includes(filters.asn.toLowerCase())
      )
    }
    
    if (filters.prefix) {
      filtered = filtered.filter(vrp => 
        vrp.prefix.includes(filters.prefix)
      )
    }
    
    setFilteredVrps(filtered)
    setPage(1)
  }

  const handleFilterChange = (key, value) => {
    setFilters(prev => ({ ...prev, [key]: value }))
  }

  const totalPages = Math.ceil(filteredVrps.length / itemsPerPage)
  const startIndex = (page - 1) * itemsPerPage
  const paginatedVrps = filteredVrps.slice(startIndex, startIndex + itemsPerPage)

  if (loading) return <div className="vrp-loading">Loading VRPs...</div>

  return (
    <div className="vrp-list">
      <div className="vrp-header">
        <h2>Validated ROA Payloads (VRPs)</h2>
        <div className="vrp-stats">
          {vrps.length > 0 && (
            <span>
              Showing {paginatedVrps.length} of {filteredVrps.length} filtered 
              (from {vrps.length} total)
            </span>
          )}
        </div>
      </div>

      <div className="filters">
        <input
          type="text"
          placeholder="Filter by ASN (e.g., AS15169)"
          value={filters.asn}
          onChange={(e) => handleFilterChange('asn', e.target.value)}
          className="filter-input"
        />
        <input
          type="text"
          placeholder="Filter by prefix (e.g., 8.8.8.0/24)"
          value={filters.prefix}
          onChange={(e) => handleFilterChange('prefix', e.target.value)}
          className="filter-input"
        />
        <button onClick={fetchVRPs} className="refresh-btn">
          Refresh
        </button>
      </div>

      {error && <div className="error-message">{error}</div>}

      {paginatedVrps.length > 0 ? (
        <>
          <div className="vrp-table-container">
            <table className="vrp-table">
              <thead>
                <tr>
                  <th>ASN</th>
                  <th>Prefix</th>
                  <th>Max Length</th>
                  <th>Trust Anchor</th>
                </tr>
              </thead>
              <tbody>
                {paginatedVrps.map((vrp, index) => (
                  <tr key={index}>
                    <td className="asn-cell">{vrp.asn}</td>
                    <td className="prefix-cell">{vrp.prefix}</td>
                    <td className="maxlength-cell">{vrp.maxLength}</td>
                    <td className="ta-cell">{vrp.ta}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {totalPages > 1 && (
            <div className="pagination">
              <button 
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page === 1}
              >
                Previous
              </button>
              <span>Page {page} of {totalPages}</span>
              <button 
                onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                disabled={page === totalPages}
              >
                Next
              </button>
            </div>
          )}
        </>
      ) : (
        <div className="no-vrps">
          {vrps.length === 0 ? 'No VRPs available' : 'No VRPs match your filters'}
        </div>
      )}
    </div>
  )
}

export default VRPList