import React, { useState } from 'react';
import { useQuery, gql } from '@apollo/client';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, LineChart, Line } from 'recharts';

const ROA_STATS_QUERY = gql`
  query GetROAStats($first: Int, $offset: Int) {
    roas(first: $first, offset: $offset, orderBy: { field: CREATED_AT, direction: DESC }) {
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
          tal {
            name
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
      totalROAs
    }
  }
`;

function ROAStats() {
  const [page, setPage] = useState(0);
  const pageSize = 10;
  const { loading, error, data } = useQuery(ROA_STATS_QUERY, {
    variables: { first: pageSize, offset: page * pageSize },
  });

  const totalPages = data ? Math.ceil(data.roas.totalCount / pageSize) : 0;

  const goToPage = (newPage) => {
    setPage(newPage);
  };

  if (loading) return <p>Loading ROA statistics...</p>;
  if (error) return <p>Error: {error.message}</p>;

  const roas = data.roas.edges.map(edge => edge.node);
  const totalROAs = data.roas.totalCount;

  // TAL distribution
  const talStats = {};
  roas.forEach(roa => {
    const tal = roa.tal?.name || 'Unknown';
    talStats[tal] = (talStats[tal] || 0) + 1;
  });

  const talData = Object.entries(talStats)
    .sort(([,a], [,b]) => b - a)
    .map(([tal, count]) => ({ name: tal, value: count }));

  // Max length distribution
  const maxLengthStats = {};
  roas.forEach(roa => {
    const maxLen = roa.maxLength || 0;
    maxLengthStats[maxLen] = (maxLengthStats[maxLen] || 0) + 1;
  });

  const maxLengthData = Object.entries(maxLengthStats)
    .sort(([a], [b]) => parseInt(a) - parseInt(b))
    .map(([length, count]) => ({ maxLength: parseInt(length), count }));

  // ASN distribution (top 10)
  const asnStats = {};
  roas.forEach(roa => {
    const asnNum = roa.asn?.number || 'Unknown';
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

  roas.forEach(roa => {
    const notBefore = new Date(roa.validity?.notBefore);
    const notAfter = new Date(roa.validity?.notAfter);

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

  return (
    <div className="stats-container">
      <h2>ROA Statistics</h2>

      <div className="summary-grid">
        <div className="summary-card">
          <h3>Total ROAs</h3>
          <p>{data.globalSummary.totalROAs}</p>
        </div>
        <div className="summary-card">
          <h3>Valid ROAs</h3>
          <p style={{ color: '#00C49F' }}>{validityStats.valid}</p>
        </div>
        <div className="summary-card">
          <h3>Expired ROAs</h3>
          <p style={{ color: '#FF8042' }}>{validityStats.expired}</p>
        </div>
        <div className="summary-card">
          <h3>Unique TALs</h3>
          <p>{Object.keys(talStats).length}</p>
        </div>
      </div>

      <div className="charts-grid">
        <div className="chart-container">
          <h3>ROA Validity Status</h3>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={validityData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="value" fill="#8884d8" />
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-container">
          <h3>Top 10 ASNs by ROA Count</h3>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={asnData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="value" fill="#82ca9d" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="charts-grid">
        <div className="chart-container">
          <h3>TAL Distribution</h3>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={talData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="value" fill="#ffc658" />
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-container">
          <h3>Max Length Distribution</h3>
          <ResponsiveContainer width="100%" height={300}>
            <LineChart data={maxLengthData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="maxLength" />
              <YAxis />
              <Tooltip />
              <Line type="monotone" dataKey="count" stroke="#8884d8" strokeWidth={2} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>

       <div className="table-container">
         <h3>ROAs ({totalROAs})</h3>
         <table className="data-table">
           <thead>
             <tr>
               <th>ASN</th>
               <th>Prefix</th>
               <th>Max Length</th>
               <th>TAL</th>
               <th>Valid From</th>
               <th>Valid To</th>
               <th>Status</th>
             </tr>
           </thead>
           <tbody>
             {roas.map(roa => {
               const notBefore = new Date(roa.validity?.notBefore);
               const notAfter = new Date(roa.validity?.notAfter);
               const isValid = now >= notBefore && now <= notAfter;

               return (
                 <tr key={roa.id}>
                   <td>{roa.asn?.number || 'N/A'}</td>
                   <td>{roa.prefix?.cidr || 'N/A'}</td>
                   <td>{roa.maxLength || 'N/A'}</td>
                   <td>{roa.tal?.name || 'N/A'}</td>
                   <td>{notBefore.toLocaleDateString()}</td>
                   <td>{notAfter.toLocaleDateString()}</td>
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
           <button onClick={() => goToPage(page + 1)} disabled={!data.roas.pageInfo.hasNextPage}>
             Next
           </button>
         </div>
       </div>
    </div>
  );
}

export default ROAStats;