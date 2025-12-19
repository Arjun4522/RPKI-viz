import React, { useState } from 'react'
import { rpkiApi } from '../services/api'

function RouteValidator() {
  const [formData, setFormData] = useState({
    asn: '',
    prefix: ''
  })
  const [validationResult, setValidationResult] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  const handleInputChange = (e) => {
    const { name, value } = e.target
    setFormData(prev => ({
      ...prev,
      [name]: value
    }))
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    
    if (!formData.asn || !formData.prefix) {
      setError('Both ASN and prefix are required')
      return
    }

    try {
      setLoading(true)
      setError(null)
      
      const response = await rpkiApi.validateRoute(formData.asn, formData.prefix)
      setValidationResult(response.data)
    } catch (err) {
      setError('Validation failed. Please check your inputs.')
      setValidationResult(null)
    } finally {
      setLoading(false)
    }
  }

  const formatASN = (asn) => {
    if (asn.startsWith('AS')) return asn
    return `AS${asn}`
  }

  return (
    <div className="route-validator">
      <h2>BGP Route Validation</h2>
      
      <form onSubmit={handleSubmit} className="validation-form">
        <div className="form-group">
          <label htmlFor="asn">AS Number:</label>
          <input
            type="text"
            id="asn"
            name="asn"
            value={formData.asn}
            onChange={handleInputChange}
            placeholder="AS15169 or 15169"
            className="form-input"
          />
        </div>

        <div className="form-group">
          <label htmlFor="prefix">IP Prefix:</label>
          <input
            type="text"
            id="prefix"
            name="prefix"
            value={formData.prefix}
            onChange={handleInputChange}
            placeholder="8.8.8.0/24"
            className="form-input"
          />
        </div>

        <button 
          type="submit" 
          disabled={loading}
          className="validate-btn"
        >
          {loading ? 'Validating...' : 'Validate Route'}
        </button>
      </form>

      {error && (
        <div className="error-message">
          {error}
        </div>
      )}

      {validationResult && (
        <div className="validation-result">
          <h3>Validation Result</h3>
          
          <div className={`result-status ${validationResult.valid ? 'valid' : 'invalid'}`}>
            {validationResult.valid ? 'VALID' : 'INVALID'}
          </div>

          <div className="result-details">
            <p><strong>ASN:</strong> {validationResult.asn}</p>
            <p><strong>Prefix:</strong> {validationResult.prefix}</p>
            <p><strong>Serial:</strong> {validationResult.serial}</p>
          </div>

          {validationResult.matching_vrps && validationResult.matching_vrps.length > 0 && (
            <div className="matching-vrps">
              <h4>Matching VRPs:</h4>
              <ul>
                {validationResult.matching_vrps.map((vrp, index) => (
                  <li key={index}>
                    {vrp.asn} - {vrp.prefix} (max: {vrp.maxLength}, TA: {vrp.ta})
                  </li>
                ))}
              </ul>
            </div>
          )}

          {!validationResult.valid && (
            <div className="invalid-reason">
              <p>No matching VRP found for this route announcement.</p>
              <p>This route would be considered INVALID by RPKI validation.</p>
            </div>
          )}
        </div>
      )}

      <div className="validator-info">
        <h4>About Route Validation</h4>
        <ul>
          <li>Validates BGP route announcements against RPKI data</li>
          <li>Returns VALID if a matching VRP exists</li>
          <li>Returns INVALID if no matching VRP found</li>
          <li>Based on RFC 6811 - BGP Prefix Origin Validation</li>
        </ul>
      </div>
    </div>
  )
}

export default RouteValidator