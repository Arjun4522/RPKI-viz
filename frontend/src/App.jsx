import React, { useState, useEffect } from 'react'
import Dashboard from './components/Dashboard'
import VRPList from './components/VRPList'
import RouteValidator from './components/RouteValidator'
import './App.css'

function App() {
  const [activeTab, setActiveTab] = useState('dashboard')
  const [healthStatus, setHealthStatus] = useState(null)

  useEffect(() => {
    checkHealth()
    const interval = setInterval(checkHealth, 30000)
    return () => clearInterval(interval)
  }, [])

  const checkHealth = async () => {
    try {
      const response = await fetch('/health')
      const data = await response.json()
      setHealthStatus(data.status === 'healthy')
    } catch (error) {
      setHealthStatus(false)
    }
  }

  const renderContent = () => {
    switch (activeTab) {
      case 'dashboard':
        return <Dashboard />
      case 'vrps':
        return <VRPList />
      case 'validator':
        return <RouteValidator />
      default:
        return <Dashboard />
    }
  }

  return (
    <div className="app">
      <header className="app-header">
        <h1>RPKI Visualization Dashboard</h1>
        <div className={`status ${healthStatus ? 'online' : 'offline'}`}>
          {healthStatus ? 'Online' : 'Offline'}
        </div>
      </header>

      <nav className="app-nav">
        <button 
          className={activeTab === 'dashboard' ? 'active' : ''}
          onClick={() => setActiveTab('dashboard')}
        >
          Dashboard
        </button>
        <button 
          className={activeTab === 'vrps' ? 'active' : ''}
          onClick={() => setActiveTab('vrps')}
        >
          VRP List
        </button>
        <button 
          className={activeTab === 'validator' ? 'active' : ''}
          onClick={() => setActiveTab('validator')}
        >
          Route Validator
        </button>
      </nav>

      <main className="app-main">
        {renderContent()}
      </main>
    </div>
  )
}

export default App