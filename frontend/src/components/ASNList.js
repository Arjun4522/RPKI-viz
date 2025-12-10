import React, { useState } from 'react';
import { useQuery, gql } from '@apollo/client';

const ASNS_QUERY = gql`
  query GetASNs($first: Int, $offset: Int) {
    asns(first: $first, offset: $offset) {
      edges {
        node {
          id
          number
          name
          country
          validationState
        }
      }
      pageInfo {
        hasNextPage
      }
      totalCount
    }
  }
`;

function ASNList() {
  const [page, setPage] = useState(0);
  const pageSize = 10;
  const { loading, error, data } = useQuery(ASNS_QUERY, {
    variables: { first: pageSize, offset: page * pageSize },
  });

  const totalPages = data ? Math.ceil(data.asns.totalCount / pageSize) : 0;

  const goToPage = (newPage) => {
    setPage(newPage);
  };

  if (loading) return <p>Loading ASNs...</p>;
  if (error) return <p>Error: {error.message}</p>;

  return (
    <div className="asn-list">
      <h3>ASNs ({data.asns.totalCount})</h3>
      <table>
        <thead>
          <tr>
            <th>ASN</th>
            <th>Name</th>
            <th>Country</th>
            <th>Validation State</th>
          </tr>
        </thead>
        <tbody>
          {data.asns.edges.map(({ node }) => (
            <tr key={node.id}>
              <td>{node.number}</td>
              <td>{node.name || 'N/A'}</td>
              <td>{node.country || 'N/A'}</td>
              <td>{node.validationState}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="pagination">
        <button onClick={() => goToPage(page - 1)} disabled={page === 0}>
          Previous
        </button>
        <span>Page {page + 1} of {totalPages}</span>
        <button onClick={() => goToPage(page + 1)} disabled={!data.asns.pageInfo.hasNextPage}>
          Next
        </button>
      </div>
    </div>
  );
}

export default ASNList;