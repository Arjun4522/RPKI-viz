import React, { useState } from 'react';
import { useLazyQuery, gql } from '@apollo/client';

const VALIDATE_PREFIX_QUERY = gql`
  query ValidatePrefix($asn: Int!, $prefix: String!) {
    validatePrefix(asn: $asn, prefix: $prefix) {
      asn
      prefix
      state
      reason
      matchedVRPs {
        id
        asn {
          number
        }
        prefix {
          cidr
        }
        maxLength
      }
    }
  }
`;

function PrefixValidator() {
  const [asn, setAsn] = useState('');
  const [prefix, setPrefix] = useState('');
  const [validatePrefix, { data, loading, error }] = useLazyQuery(VALIDATE_PREFIX_QUERY);

  const handleSubmit = (e) => {
    e.preventDefault();
    validatePrefix({ variables: { asn: parseInt(asn), prefix } });
  };

  return (
    <div className="prefix-validator">
      <h3>Prefix Validation</h3>
      <form onSubmit={handleSubmit}>
        <input
          type="number"
          placeholder="ASN"
          value={asn}
          onChange={(e) => setAsn(e.target.value)}
          required
        />
        <input
          type="text"
          placeholder="Prefix (e.g., 192.0.2.0/24)"
          value={prefix}
          onChange={(e) => setPrefix(e.target.value)}
          required
        />
        <button type="submit" disabled={loading}>
          {loading ? 'Validating...' : 'Validate'}
        </button>
      </form>

      {error && <p>Error: {error.message}</p>}

      {data && (
        <div className="validation-result">
          <h4>Validation Result</h4>
          <p><strong>ASN:</strong> {data.validatePrefix.asn}</p>
          <p><strong>Prefix:</strong> {data.validatePrefix.prefix}</p>
          <p><strong>State:</strong> {data.validatePrefix.state}</p>
          {data.validatePrefix.reason && <p><strong>Reason:</strong> {data.validatePrefix.reason}</p>}
          {data.validatePrefix.matchedVRPs.length > 0 && (
            <div>
              <h5>Matched VRPs:</h5>
              <ul>
                {data.validatePrefix.matchedVRPs.map((vrp) => (
                  <li key={vrp.id}>
                    ASN {vrp.asn.number}, {vrp.prefix.cidr}, Max Length: {vrp.maxLength}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default PrefixValidator;