import axios from 'axios'

const API_BASE_URL = 'http://localhost:8080'

const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000
})

export const rpkiApi = {
  getHealth: () => api.get('/health'),
  
  getMetrics: () => api.get('/metrics'),
  
  getState: () => api.get('/api/v1/state'),
  
  getVRPs: (filters = {}) => {
    const params = new URLSearchParams()
    if (filters.asn) params.append('asn', filters.asn)
    if (filters.prefix) params.append('prefix', filters.prefix)
    return api.get(`/api/v1/vrps?${params.toString()}`)
  },
  
  getDiff: (fromSerial, toSerial) => 
    api.get(`/api/v1/diff?from=${fromSerial}&to=${toSerial}`),
  
  validateRoute: (asn, prefix) => 
    api.post('/api/v1/validate', { asn, prefix })
}