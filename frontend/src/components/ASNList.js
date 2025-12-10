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
  const [offset, setOffset] = useState(0);
  const { loading, error, data, fetchMore } = useQuery(ASNS_QUERY, {
    variables: { first: 20, offset: 0 },
  });

  const loadMore = () => {
    fetchMore({
      variables: {
        offset: offset + 20,
      },
      updateQuery: (prev, { fetchMoreResult }) => {
        if (!fetchMoreResult) return prev;
        return {
          ...prev,
          asns: {
            ...prev.asns,
            edges: [...prev.asns.edges, ...fetchMoreResult.asns.edges],
            pageInfo: fetchMoreResult.asns.pageInfo,
          },
        };
      },
    });
    setOffset(offset + 20);
  };

  if (loading && !data) return <p>Loading ASNs...</p>;
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
      {data.asns.pageInfo.hasNextPage && (
        <button onClick={loadMore}>Load More</button>
      )}
    </div>
  );
}

export default ASNList;