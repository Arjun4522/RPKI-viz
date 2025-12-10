import React, { useState } from 'react';
import { useQuery, gql } from '@apollo/client';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts';

const VRP_STATS_QUERY = gql`
  query GetVRPStats($first: Int, $offset: Int) {
    vrps(first: $first, offset: $offset, orderBy: { field: CREATED_AT, direction: DESC }) {
      edges {
        node {
          id
          asn {
            number
            name
          }
          prefix {
            cidr
          }
          maxLength
          validity {
            notBefore
            notAfter
          }
          roa {
            id
          }
          createdAt
        }
      }
      pageInfo {
        hasNextPage
      }
      totalCount
    }
    globalSummary {
      totalVRPs
    }
  }
`;

function VRPStats() {
  const [page, setPage] = useState(0);
  const pageSize = 10;
  const { loading, error, data } = useQuery(VRP_STATS_QUERY, {
    variables: { first: pageSize, offset: page * pageSize },
  });

  const totalPages = data ? Math.ceil(data.vrps.totalCount / pageSize) : 0;

  const goToPage = (newPage) => {
    setPage(newPage);
  };

  if (loading) return <p>Loading VRP statistics...</p>;
  if (error) return <p>Error: {error.message}</p>;

  const vrps = data.vrps.edges.map(edge => edge.node);
  const totalVRPs = data.vrps.totalCount;

  // Max length distribution
  const maxLengthStats = {};
  vrps.forEach(vrp => {
    const maxLen = vrp.maxLength || 0;
    maxLengthStats[maxLen] = (maxLengthStats[maxLen] || 0) + 1;
  });

  const maxLengthData = Object.entries(maxLengthStats)
    .sort(([a], [b]) => parseInt(a) - parseInt(b))
    .map(([length, count]) => ({ maxLength: parseInt(length), count }));

  // ASN distribution (top 10)
  const asnStats = {};
  vrps.forEach(vrp => {
    const asnNum = vrp.asn?.number || 'Unknown';
    asnStats[asnNum] = (asnStats[asnNum] || 0) + 1;
  });

  const asnData = Object.entries(asnStats)
    .sort(([,a], [,b]) => b - a)
    .slice(0, 10)
    .map(([asn, count]) => ({ name: `AS${asn}`, value: count }));

  // Validity period analysis
  const now = new Date();
  const validityStats = {
    valid: 0,
    expired: 0,
    notYetValid: 0,
  };

  vrps.forEach(vrp => {
    const notBefore = new Date(vrp.validity?.notBefore);
    const notAfter = new Date(vrp.validity?.notAfter);

    if (now >= notBefore && now <= notAfter) {
      validityStats.valid++;
    } else if (now < notBefore) {
      validityStats.notYetValid++;
    } else {
      validityStats.expired++;
    }
  });

  const validityData = [
    { name: 'Valid', value: validityStats.valid, color: '#00C49F' },
    { name: 'Expired', value: validityStats.expired, color: '#FF8042' },
    { name: 'Not Yet Valid', value: validityStats.notYetValid, color: '#FFBB28' },
  ];

  // ROA coverage
  const withROA = vrps.filter(vrp => vrp.roa).length;
  const withoutROA = vrps.length - withROA;

  const roaCoverageData = [
    { name: 'With ROA', value: withROA, color: '#00C49F' },
    { name: 'Without ROA', value: withoutROA, color: '#FF8042' },
  ];

  return (
    <div className="stats-container">
      <h2>VRP Statistics</h2>

      <div className="summary-grid">
        <div className="summary-card">
          <h3>Total VRPs</h3>
          <p>{data.globalSummary.totalVRPs}</p>
        </div>
        <div className="summary-card">
          <h3>Valid VRPs</h3>
          <p style={{ color: '#00C49F' }}>{validityStats.valid}</p>
        </div>
        <div className="summary-card">
          <h3>Expired VRPs</h3>
          <p style={{ color: '#FF8042' }}>{validityStats.expired}</p>
        </div>
        <div className="summary-card">
          <h3>With ROA Reference</h3>
          <p>{withROA}</p>
        </div>
      </div>

      <div className="charts-grid">
        <div className="chart-container">
          <h3>VRP Validity Status</h3>
          <ResponsiveContainer width="100%" height={300}>
            <PieChart>
              <Pie
                data={validityData}
                cx="50%"
                cy="50%"
                labelLine={false}
                label={({ name, percent }) => `${name}: ${(percent * 100).toFixed(0)}%`}
                outerRadius={80}
                fill="#8884d8"
                dataKey="value"
              >
                {validityData.map((entry, index) => (
                  <Cell key={`cell-${index}`} fill={entry.color} />
                ))}
              </Pie>
              <Tooltip />
            </PieChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-container">
          <h3>ROA Coverage</h3>
          <ResponsiveContainer width="100%" height={300}>
            <PieChart>
              <Pie
                data={roaCoverageData}
                cx="50%"
                cy="50%"
                labelLine={false}
                label={({ name, percent }) => `${name}: ${(percent * 100).toFixed(0)}%`}
                outerRadius={80}
                fill="#8884d8"
                dataKey="value"
              >
                {roaCoverageData.map((entry, index) => (
                  <Cell key={`cell-${index}`} fill={entry.color} />
                ))}
              </Pie>
              <Tooltip />
            </PieChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="charts-grid">
        <div className="chart-container">
          <h3>Top 10 ASNs by VRP Count</h3>
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

        <div className="chart-container">
          <h3>Max Length Distribution</h3>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={maxLengthData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="maxLength" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="value" fill="#82ca9d" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>

       <div className="table-container">
         <h3>VRPs ({totalVRPs})</h3>
         <table className="data-table">
           <thead>
             <tr>
               <th>ASN</th>
               <th>Prefix</th>
               <th>Max Length</th>
               <th>Valid From</th>
               <th>Valid To</th>
               <th>Has ROA</th>
               <th>Status</th>
             </tr>
           </thead>
           <tbody>
             {vrps.map(vrp => {
               const notBefore = new Date(vrp.validity?.notBefore);
               const notAfter = new Date(vrp.validity?.notAfter);
               const isValid = now >= notBefore && now <= notAfter;

               return (
                 <tr key={vrp.id}>
                   <td>{vrp.asn?.number || 'N/A'}</td>
                   <td>{vrp.prefix?.cidr || 'N/A'}</td>
                   <td>{vrp.maxLength || 'N/A'}</td>
                   <td>{notBefore.toLocaleDateString()}</td>
                   <td>{notAfter.toLocaleDateString()}</td>
                   <td>{vrp.roa ? 'Yes' : 'No'}</td>
                   <td>
                     <span className={isValid ? 'status-valid' : 'status-invalid'}>
                       {isValid ? 'Valid' : 'Invalid'}
                     </span>
                   </td>
                 </tr>
               );
             })}
           </tbody>
         </table>
         <div className="pagination">
           <button onClick={() => goToPage(page - 1)} disabled={page === 0}>
             Previous
           </button>
           <span>Page {page + 1} of {totalPages}</span>
           <button onClick={() => goToPage(page + 1)} disabled={!data.vrps.pageInfo.hasNextPage}>
             Next
           </button>
         </div>
       </div>
    </div>
  );
}

export default VRPStats;