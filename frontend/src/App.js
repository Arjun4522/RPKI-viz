import React from 'react';
import { ApolloClient, InMemoryCache, ApolloProvider } from '@apollo/client';
import Dashboard from './components/Dashboard';
import './App.css';

const client = new ApolloClient({
  uri: '/graphql',
  cache: new InMemoryCache()
});

function App() {
  return (
    <ApolloProvider client={client}>
      <div className="App">
        <header className="App-header">
          <h1>RPKI Visualization Platform</h1>
        </header>
        <main>
          <Dashboard />
        </main>
      </div>
    </ApolloProvider>
  );
}

export default App;