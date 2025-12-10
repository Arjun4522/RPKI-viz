import React from 'react';
import { useQuery, gql } from '@apollo/client';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts';

const ASN_STATS_QUERY = gql`
  query GetASNStats {
    asns(first: 100, orderBy: { field: NUMBER, direction: ASC }) {
      edges {
        node {
          id
          number
          name
          country
          prefixes {
            id
          }
          roas {
            id
          }
          vrps {
            id
          }
        }
      }
    }
  }
`;

function ASNStats() {
  const { loading, error, data } = useQuery(ASN_STATS_QUERY);

  if (loading) return <p>Loading ASN statistics...</p>;
  if (error) return <p>Error: {error.message}</p>;

  const asns = data.asns.edges.map(edge => edge.node);

  // Calculate statistics
  const totalASNs = asns.length;
  const asnsWithPrefixes = asns.filter(asn => asn.prefixes.length > 0).length;
  const asnsWithROAs = asns.filter(asn => asn.roas.length > 0).length;
  const asnsWithVRPs = asns.filter(asn => asn.vrps.length > 0).length;

  // Country distribution
  const countryStats = {};
  asns.forEach(asn => {
    const country = asn.country || 'Unknown';
    countryStats[country] = (countryStats[country] || 0) + 1;
  });

  const countryData = Object.entries(countryStats)
    .sort(([,a], [,b]) => b - a)
    .slice(0, 10)
    .map(([country, count]) => ({ name: country, value: count }));

  // ASN size distribution (by prefix count)
  const sizeDistribution = [
    { range: '0', count: asns.filter(asn => asn.prefixes.length === 0).length },
    { range: '1-5', count: asns.filter(asn => asn.prefixes.length >= 1 && asn.prefixes.length <= 5).length },
    { range: '6-20', count: asns.filter(asn => asn.prefixes.length >= 6 && asn.prefixes.length <= 20).length },
    { range: '21-100', count: asns.filter(asn => asn.prefixes.length >= 21 && asn.prefixes.length <= 100).length },
    { range: '100+', count: asns.filter(asn => asn.prefixes.length > 100).length },
  ];

  const COLORS = ['#0088FE', '#00C49F', '#FFBB28', '#FF8042', '#8884D8'];

  return (
    <div className="stats-container">
      <h2>ASN Statistics</h2>

      <div className="summary-grid">
        <div className="summary-card">
          <h3>Total ASNs</h3>
          <p>{totalASNs}</p>
        </div>
        <div className="summary-card">
          <h3>ASNs with Prefixes</h3>
          <p>{asnsWithPrefixes}</p>
        </div>
        <div className="summary-card">
          <h3>ASNs with ROAs</h3>
          <p>{asnsWithROAs}</p>
        </div>
        <div className="summary-card">
          <h3>ASNs with VRPs</h3>
          <p>{asnsWithVRPs}</p>
        </div>
      </div>

      <div className="charts-grid">
        <div className="chart-container">
          <h3>Top 10 Countries by ASN Count</h3>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={countryData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="value" fill="#8884d8" />
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-container">
          <h3>ASN Size Distribution (by Prefix Count)</h3>
          <ResponsiveContainer width="100%" height={300}>
            <PieChart>
              <Pie
                data={sizeDistribution}
                cx="50%"
                cy="50%"
                labelLine={false}
                label={({ range, percent }) => `${range}: ${(percent * 100).toFixed(0)}%`}
                outerRadius={80}
                fill="#8884d8"
                dataKey="count"
              >
                {sizeDistribution.map((entry, index) => (
                  <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                ))}
              </Pie>
              <Tooltip />
            </PieChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="table-container">
        <h3>ASN List (First 100)</h3>
        <table className="data-table">
          <thead>
            <tr>
              <th>ASN</th>
              <th>Name</th>
              <th>Country</th>
              <th>Prefixes</th>
              <th>ROAs</th>
              <th>VRPs</th>
            </tr>
          </thead>
          <tbody>
            {asns.map(asn => (
              <tr key={asn.id}>
                <td>{asn.number}</td>
                <td>{asn.name || 'N/A'}</td>
                <td>{asn.country || 'Unknown'}</td>
                <td>{asn.prefixes.length}</td>
                <td>{asn.roas.length}</td>
                <td>{asn.vrps.length}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export default ASNStats;