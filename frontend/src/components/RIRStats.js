import React from 'react';
import { useQuery, gql } from '@apollo/client';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';

const RIR_STATS_QUERY = gql`
  query GetRIRStats {
    globalSummary {
      rirStats {
        rir
        totalROAs
        totalVRPs
        validPrefixes
        invalidPrefixes
        notFoundPrefixes
      }
    }
  }
`;

function RIRStats() {
  const { loading, error, data } = useQuery(RIR_STATS_QUERY);

  if (loading) return <p>Loading RIR statistics...</p>;
  if (error) return <p>Error: {error.message}</p>;

  const rirStats = data.globalSummary.rirStats;

  // Prepare data for charts
  const roaData = rirStats.map(stat => ({
    name: stat.rir,
    ROAs: stat.totalROAs,
  }));

  const vrpData = rirStats.map(stat => ({
    name: stat.rir,
    VRPs: stat.totalVRPs,
  }));

  const prefixData = rirStats.map(stat => ({
    name: stat.rir,
    Valid: stat.validPrefixes,
    Invalid: stat.invalidPrefixes,
    'Not Found': stat.notFoundPrefixes,
  }));

  // Calculate totals
  const totals = rirStats.reduce((acc, stat) => ({
    totalROAs: acc.totalROAs + stat.totalROAs,
    totalVRPs: acc.totalVRPs + stat.totalVRPs,
    totalValid: acc.totalValid + stat.validPrefixes,
    totalInvalid: acc.totalInvalid + stat.invalidPrefixes,
    totalNotFound: acc.totalNotFound + stat.notFoundPrefixes,
  }), { totalROAs: 0, totalVRPs: 0, totalValid: 0, totalInvalid: 0, totalNotFound: 0 });

  return (
    <div className="stats-container">
      <h2>RIR Statistics</h2>

      <div className="summary-grid">
        <div className="summary-card">
          <h3>Total RIRs</h3>
          <p>{rirStats.length}</p>
        </div>
        <div className="summary-card">
          <h3>Total ROAs</h3>
          <p>{totals.totalROAs}</p>
        </div>
        <div className="summary-card">
          <h3>Total VRPs</h3>
          <p>{totals.totalVRPs}</p>
        </div>
        <div className="summary-card">
          <h3>Total Prefixes</h3>
          <p>{totals.totalValid + totals.totalInvalid + totals.totalNotFound}</p>
        </div>
      </div>

      <div className="charts-grid">
        <div className="chart-container">
          <h3>ROAs by RIR</h3>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={roaData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="ROAs" fill="#8884d8" />
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-container">
          <h3>VRPs by RIR</h3>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={vrpData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="VRPs" fill="#82ca9d" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="chart-container">
        <h3>Prefix Validation by RIR</h3>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={prefixData}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="name" />
            <YAxis />
            <Tooltip />
            <Legend />
            <Bar dataKey="Valid" stackId="a" fill="#00C49F" />
            <Bar dataKey="Invalid" stackId="a" fill="#FF8042" />
            <Bar dataKey="Not Found" stackId="a" fill="#FFBB28" />
          </BarChart>
        </ResponsiveContainer>
      </div>

      <div className="table-container">
        <h3>RIR Statistics Table</h3>
        <table className="data-table">
          <thead>
            <tr>
              <th>RIR</th>
              <th>ROAs</th>
              <th>VRPs</th>
              <th>Valid Prefixes</th>
              <th>Invalid Prefixes</th>
              <th>Not Found Prefixes</th>
              <th>Total Prefixes</th>
            </tr>
          </thead>
          <tbody>
            {rirStats.map(stat => (
              <tr key={stat.rir}>
                <td>{stat.rir}</td>
                <td>{stat.totalROAs.toLocaleString()}</td>
                <td>{stat.totalVRPs.toLocaleString()}</td>
                <td style={{ color: '#00C49F' }}>{stat.validPrefixes.toLocaleString()}</td>
                <td style={{ color: '#FF8042' }}>{stat.invalidPrefixes.toLocaleString()}</td>
                <td style={{ color: '#FFBB28' }}>{stat.notFoundPrefixes.toLocaleString()}</td>
                <td>{(stat.validPrefixes + stat.invalidPrefixes + stat.notFoundPrefixes).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr style={{ fontWeight: 'bold', backgroundColor: '#f5f5f5' }}>
              <td>Total</td>
              <td>{totals.totalROAs.toLocaleString()}</td>
              <td>{totals.totalVRPs.toLocaleString()}</td>
              <td style={{ color: '#00C49F' }}>{totals.totalValid.toLocaleString()}</td>
              <td style={{ color: '#FF8042' }}>{totals.totalInvalid.toLocaleString()}</td>
              <td style={{ color: '#FFBB28' }}>{totals.totalNotFound.toLocaleString()}</td>
              <td>{(totals.totalValid + totals.totalInvalid + totals.totalNotFound).toLocaleString()}</td>
            </tr>
          </tfoot>
        </table>
      </div>
    </div>
  );
}

export default RIRStats;