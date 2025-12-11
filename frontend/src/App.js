import React, { useState } from 'react';
import { ApolloClient, InMemoryCache, ApolloProvider } from '@apollo/client';
import Sidebar from './components/Sidebar';
import Dashboard from './components/Dashboard';
import ASNStats from './components/ASNStats';
import PrefixStats from './components/PrefixStats';
import ROAStats from './components/ROAStats';
import VRPStats from './components/VRPStats';
import RIRStats from './components/RIRStats';
import './App.css';

const client = new ApolloClient({
  uri: 'http://localhost:8080/graphql',
  cache: new InMemoryCache()
});

function App() {
  const [activeView, setActiveView] = useState('global');

  const renderContent = () => {
    switch (activeView) {
      case 'global':
        return <Dashboard />;
      case 'asns':
        return <ASNStats />;
      case 'prefixes':
        return <PrefixStats />;
      case 'roas':
        return <ROAStats />;
      case 'vrps':
        return <VRPStats />;
      case 'rir':
        return <RIRStats />;
      case 'validation':
        return <Dashboard />; // For now, reuse dashboard which has validation
      default:
        return <Dashboard />;
    }
  };

  return (
    <ApolloProvider client={client}>
      <div className="App">
        <Sidebar activeView={activeView} onViewChange={setActiveView} />
        <div className="main-content">
          <header className="App-header">
            <h1>RPKI Visualization Platform</h1>
          </header>
          <main>
            {renderContent()}
          </main>
        </div>
      </div>
    </ApolloProvider>
  );
}

export default App;