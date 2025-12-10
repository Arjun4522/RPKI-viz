import React from 'react';
import { useQuery, gql } from '@apollo/client';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import PrefixValidator from './PrefixValidator';
import './Dashboard.css';

const GLOBAL_SUMMARY_QUERY = gql`
  query GetGlobalSummary {
    globalSummary {
      totalASNs
      totalPrefixes
      totalROAs
      totalVRPs
      validPrefixes
      invalidPrefixes
      notFoundPrefixes
    }
  }
`;

function Dashboard() {
  const { loading, error, data } = useQuery(GLOBAL_SUMMARY_QUERY);

  if (loading) return <p>Loading...</p>;
  if (error) return <p>Error: {error.message}</p>;

  const summary = data.globalSummary;
  const totalsData = [
    { name: 'ASNs', value: summary.totalASNs },
    { name: 'Prefixes', value: summary.totalPrefixes },
    { name: 'ROAs', value: summary.totalROAs },
    { name: 'VRPs', value: summary.totalVRPs },
  ];

  return (
    <div className="dashboard">
      <h2>Global Summary</h2>
      <div className="summary-grid">
        <div className="summary-card">
          <h3>Total ASNs</h3>
          <p>{summary.totalASNs}</p>
        </div>
        <div className="summary-card">
          <h3>Total Prefixes</h3>
          <p>{summary.totalPrefixes}</p>
        </div>
        <div className="summary-card">
          <h3>Total ROAs</h3>
          <p>{summary.totalROAs}</p>
        </div>
        <div className="summary-card">
          <h3>Total VRPs</h3>
          <p>{summary.totalVRPs}</p>
        </div>
      </div>





       <h3>Total Counts</h3>
       <ResponsiveContainer width="100%" height={300}>
         <BarChart data={totalsData}>
           <CartesianGrid strokeDasharray="3 3" />
           <XAxis dataKey="name" />
           <YAxis />
           <Tooltip />
           <Legend />
           <Bar dataKey="value" fill="#82ca9d" />
         </BarChart>
       </ResponsiveContainer>

        <PrefixValidator />
    </div>
  );
}

export default Dashboard;