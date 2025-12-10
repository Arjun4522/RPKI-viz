import React from 'react';
import { useQuery, gql } from '@apollo/client';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts';

const PREFIX_STATS_QUERY = gql`
  query GetPrefixStats {
    prefixes(first: 1000, orderBy: { field: CIDR, direction: ASC }) {
      edges {
        node {
          id
          cidr
          asn {
            number
            name
          }
          validationState
          maxLength
        }
      }
    }
    globalSummary {
      validPrefixes
      invalidPrefixes
      notFoundPrefixes
    }
  }
`;

function PrefixStats() {
  const { loading, error, data } = useQuery(PREFIX_STATS_QUERY);

  if (loading) return <p>Loading prefix statistics...</p>;
  if (error) return <p>Error: {error.message}</p>;

  const prefixes = data.prefixes.edges.map(edge => edge.node);
  const summary = data.globalSummary;

  // Validation state distribution
  const validationData = [
    { name: 'Valid', value: summary.validPrefixes, color: '#00C49F' },
    { name: 'Invalid', value: summary.invalidPrefixes, color: '#FF8042' },
    { name: 'Not Found', value: summary.notFoundPrefixes, color: '#FFBB28' },
  ];

  // ASN distribution (top 10 by prefix count)
  const asnStats = {};
  prefixes.forEach(prefix => {
    const asnNum = prefix.asn?.number || 'Unknown';
    asnStats[asnNum] = (asnStats[asnNum] || 0) + 1;
  });

  const asnData = Object.entries(asnStats)
    .sort(([,a], [,b]) => b - a)
    .slice(0, 10)
    .map(([asn, count]) => ({ name: `AS${asn}`, value: count }));

  // Max length distribution
  const maxLengthStats = {};
  prefixes.forEach(prefix => {
    const maxLen = prefix.maxLength || 'null';
    maxLengthStats[maxLen] = (maxLengthStats[maxLen] || 0) + 1;
  });

  const maxLengthData = Object.entries(maxLengthStats)
    .sort(([a], [b]) => {
      if (a === 'null') return 1;
      if (b === 'null') return -1;
      return parseInt(a) - parseInt(b);
    })
    .map(([length, count]) => ({ name: length === 'null' ? 'Not Set' : `/${length}`, value: count }));

  return (
    <div className="stats-container">
      <h2>Prefix Statistics</h2>

      <div className="summary-grid">
        <div className="summary-card">
          <h3>Total Prefixes</h3>
          <p>{summary.validPrefixes + summary.invalidPrefixes + summary.notFoundPrefixes}</p>
        </div>
        <div className="summary-card">
          <h3>Valid Prefixes</h3>
          <p style={{ color: '#00C49F' }}>{summary.validPrefixes}</p>
        </div>
        <div className="summary-card">
          <h3>Invalid Prefixes</h3>
          <p style={{ color: '#FF8042' }}>{summary.invalidPrefixes}</p>
        </div>
        <div className="summary-card">
          <h3>Not Found Prefixes</h3>
          <p style={{ color: '#FFBB28' }}>{summary.notFoundPrefixes}</p>
        </div>
      </div>

      <div className="charts-grid">
        <div className="chart-container">
          <h3>Prefix Validation State Distribution</h3>
          <ResponsiveContainer width="100%" height={300}>
            <PieChart>
              <Pie
                data={validationData}
                cx="50%"
                cy="50%"
                labelLine={false}
                label={({ name, percent }) => `${name}: ${(percent * 100).toFixed(0)}%`}
                outerRadius={80}
                fill="#8884d8"
                dataKey="value"
              >
                {validationData.map((entry, index) => (
                  <Cell key={`cell-${index}`} fill={entry.color} />
                ))}
              </Pie>
              <Tooltip />
            </PieChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-container">
          <h3>Top 10 ASNs by Prefix Count</h3>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={asnData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="value" fill="#8884d8" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="chart-container">
        <h3>Max Length Distribution</h3>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={maxLengthData}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="name" />
            <YAxis />
            <Tooltip />
            <Bar dataKey="value" fill="#82ca9d" />
          </BarChart>
        </ResponsiveContainer>
      </div>

      <div className="table-container">
        <h3>Prefix List (First 1000)</h3>
        <table className="data-table">
          <thead>
            <tr>
              <th>CIDR</th>
              <th>ASN</th>
              <th>ASN Name</th>
              <th>Validation State</th>
              <th>Max Length</th>
            </tr>
          </thead>
          <tbody>
            {prefixes.slice(0, 100).map(prefix => (
              <tr key={prefix.id}>
                <td>{prefix.cidr}</td>
                <td>{prefix.asn?.number || 'N/A'}</td>
                <td>{prefix.asn?.name || 'N/A'}</td>
                <td>
                  <span className={`status-${prefix.validationState.toLowerCase()}`}>
                    {prefix.validationState}
                  </span>
                </td>
                <td>{prefix.maxLength || 'Not Set'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export default PrefixStats;