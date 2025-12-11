# RPKI Visualization Platform Backend

A production-grade Go backend for RPKI (Resource Public Key Infrastructure) data visualization and monitoring.

## Features

- **GraphQL API** for querying RPKI data (ASNs, prefixes, ROAs, VRPs)
- **Multi-source ingestion** from RIPE NCC, Cloudflare, and NLnet Labs Routinator
- **RRDP and rsync fetchers** for real-time RPKI data updates
- **Prefix validation engine** with VRP-based validation
- **Redis caching** for performance optimization
- **PostgreSQL storage** for persistent data
- **Docker deployment** with docker-compose

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   RIPE NCC      │    │  Cloudflare     │    │  Routinator     │
│   RRDP/rsync    │    │   JSON Feed     │    │   HTTP API      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌────────────────────┐
                    │   Ingestion        │
                    │   Pipelines        │
                    └────────────────────┘
                                 │
                    ┌────────────────────┐
                    │   Processing       │
                    │   ROA → VRP        │
                    │   Validation       │
                    └────────────────────┘
                                 │
                    ┌────────────────────┐
                    │   PostgreSQL       │
                    │   + Redis Cache    │
                    └────────────────────┘
                                 │
                    ┌────────────────────┐
                    │   GraphQL API      │
                    │   (gqlgen)         │
                    └────────────────────┘
```

## Quick Start

### Using Docker Compose (Recommended)

1. Clone the repository
2. Start the services:
   ```bash
   docker-compose up -d
   ```

3. The GraphQL API will be available at `http://localhost:8080/graphql`
4. Health check endpoint: `http://localhost:8080/health`

### Manual Setup

1. Install dependencies:
   ```bash
   go mod download
   ```

2. Set environment variables:
   ```bash
   export DATABASE_URL="postgres://user:password@localhost:5432/rpki_viz?sslmode=disable"
   export REDIS_URL="redis://localhost:6379"
   export SERVER_ADDR=":8080"
   ```

3. Run database migrations (PostgreSQL):
   ```bash
   psql -f db/migrations/001_create_tables.sql
   ```

4. Build and run:
   ```bash
   go build -o rpki-viz ./cmd/rpki-viz
   ./rpki-viz
   ```

## GraphQL API Guide

### Introduction to GraphQL

GraphQL is a query language for APIs that allows clients to request exactly the data they need. Unlike traditional REST APIs that have fixed endpoints returning fixed data structures, GraphQL lets you:

- **Ask for what you want**: Specify exactly which fields you need
- **Get predictable results**: The response structure matches your query
- **Fetch related data efficiently**: Get multiple related pieces of data in one request
- **Use a single endpoint**: All requests go to `/graphql`

#### Key Concepts

- **Query**: A request for data (like a GET in REST)
- **Mutation**: A request to modify data (like POST/PUT in REST) - not used in this API
- **Schema**: The blueprint defining what data is available and how to query it
- **Fields**: Individual pieces of data (like `name`, `number`)
- **Types**: Data structures (like `ASN`, `Prefix`)

### API Endpoint

The GraphQL API is available at:
```
http://localhost:8080/graphql
```

For development, you can also access the GraphQL Playground at:
```
http://localhost:8080/playground
```

The Playground is a web interface where you can explore the API, write queries, and see results in real-time.

### Making Requests

#### Using curl

```bash
curl -X POST http://localhost:8080/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "query { globalSummary { totalASNs } }"}'
```

#### Using JavaScript (Apollo Client)

```javascript
import { ApolloClient, InMemoryCache, gql } from '@apollo/client';

const client = new ApolloClient({
  uri: 'http://localhost:8080/graphql',
  cache: new InMemoryCache()
});

const query = gql`
  query {
    globalSummary {
      totalASNs
    }
  }
`;

const result = await client.query({ query });
```

### Schema Overview

The API organizes RPKI data into these main types:

- **ASN**: Autonomous System Number information
- **Prefix**: IP address ranges in CIDR notation
- **ROA**: Route Origin Authorization certificates
- **VRP**: Validated ROA Payload (processed ROA data)
- **TrustAnchor**: RPKI trust anchor information
- **ValidationResponse**: Results of prefix validation

### Basic Queries

#### Getting Started - Global Summary

The simplest query shows overall statistics:

```graphql
query {
  globalSummary {
    totalASNs
    totalPrefixes
    validPrefixes
    invalidPrefixes
    notFoundPrefixes
  }
}
```

**Response:**
```json
{
  "data": {
    "globalSummary": {
      "totalASNs": 12345,
      "totalPrefixes": 67890,
      "validPrefixes": 65000,
      "invalidPrefixes": 2000,
      "notFoundPrefixes": 890
    }
  }
}
```

#### Querying a Single ASN

Get information about a specific ASN:

```graphql
query {
  asn(number: 3333) {
    number
    name
    country
    validationState
  }
}
```

**Response:**
```json
{
  "data": {
    "asn": {
      "number": 3333,
      "name": "RIPE-NCC-LEGACY-MNT",
      "country": "NL",
      "validationState": "VALID"
    }
  }
}
```

#### Listing ASNs with Pagination

Get a list of ASNs with pagination:

```graphql
query {
  asns(first: 10, offset: 0) {
    edges {
      node {
        number
        name
        validationState
      }
    }
    pageInfo {
      hasNextPage
      hasPreviousPage
    }
    totalCount
  }
}
```

**Response:**
```json
{
  "data": {
    "asns": {
      "edges": [
        {
          "node": {
            "number": 1,
            "name": "LVLT-1",
            "validationState": "VALID"
          }
        }
      ],
      "pageInfo": {
        "hasNextPage": true,
        "hasPreviousPage": false
      },
      "totalCount": 12345
    }
  }
}
```

#### Filtering ASNs

Find ASNs with specific criteria:

```graphql
query {
  asns(
    first: 5
    filter: {
      country: "US"
      hasROAs: true
      validationState: VALID
    }
  ) {
    edges {
      node {
        number
        name
        country
      }
    }
  }
}
```

#### Getting Related Data

ASNs contain related prefixes, ROAs, and VRPs:

```graphql
query {
  asn(number: 3333) {
    number
    name
    prefixes {
      cidr
      validationState
    }
    roas {
      prefix {
        cidr
      }
      maxLength
    }
    vrps {
      prefix {
        cidr
      }
      maxLength
    }
  }
}
```

#### Prefix Validation

The most important feature - validate if a prefix is valid for an ASN:

```graphql
query {
  validatePrefix(asn: 13335, prefix: "1.0.0.0/24") {
    asn
    prefix
    state
    reason
    matchedVRPs {
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
```

**Response:**
```json
{
  "data": {
    "validatePrefix": {
      "asn": 13335,
      "prefix": "1.0.0.0/24",
      "state": "VALID",
      "reason": null,
      "matchedVRPs": [
        {
          "asn": {
            "number": 13335
          },
          "prefix": {
            "cidr": "1.0.0.0/24"
          },
          "maxLength": 24
        }
      ]
    }
  }
}
```

### Understanding Types

#### ValidationState

Possible values:
- `VALID`: Prefix is correctly authorized
- `INVALID`: Prefix conflicts with RPKI data
- `NOT_FOUND`: No RPKI data found for this prefix
- `UNKNOWN`: Unable to determine validity

#### OrderDirection

For sorting results:
- `ASC`: Ascending (A-Z, 1-9)
- `DESC`: Descending (Z-A, 9-1)

#### Pagination

All list queries use "connections" pattern:
- `first`: How many items to get
- `offset`: How many to skip (for pagination)
- `edges`: Array of results with metadata
- `pageInfo`: Information about pagination state
- `totalCount`: Total number of items available

### Advanced Queries

#### Complex Filtering

```graphql
query {
  prefixes(
    first: 20
    filter: {
      asn: 3333
      validationState: VALID
      hasROA: true
    }
    orderBy: {
      field: CIDR
      direction: ASC
    }
  ) {
    edges {
      node {
        cidr
        asn {
          number
          name
        }
        validationState
      }
    }
  }
}
```

#### Getting RIR Statistics

```graphql
query {
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
```

### Best Practices

#### 1. Request Only What You Need

Instead of getting all fields:
```graphql
# ❌ Bad - gets everything
query {
  asn(number: 3333) {
    number
    name
    country
    prefixes {
      cidr
      asn
      roa
      vrp
      validationState
      maxLength
      expiresAt
      createdAt
      updatedAt
    }
    roas {
      id
      asn
      prefix
      maxLength
      validity
      certificates
      signature
      tal
      createdAt
      updatedAt
    }
    vrps {
      id
      asn
      prefix
      maxLength
      validity
      roa
      createdAt
      updatedAt
    }
    validationState
    createdAt
    updatedAt
  }
}
```

Do this:
```graphql
# ✅ Good - gets only what you need
query {
  asn(number: 3333) {
    number
    name
    validationState
    prefixes {
      cidr
      validationState
    }
  }
}
```

#### 2. Use Pagination

Don't fetch thousands of records at once:
```graphql
# ✅ Good - paginate
query {
  asns(first: 50, offset: 0) {
    edges {
      node {
        number
        name
      }
    }
    pageInfo {
      hasNextPage
    }
  }
}
```

#### 3. Batch Related Queries

Instead of multiple requests, get related data in one query:
```graphql
# ✅ Good - one query for related data
query {
  asn(number: 3333) {
    number
    name
    prefixes(first: 10) {
      edges {
        node {
          cidr
          validationState
        }
      }
    }
    roas(first: 10) {
      edges {
        node {
          prefix {
            cidr
          }
          maxLength
        }
      }
    }
  }
}
```

#### 4. Use Variables for Dynamic Values

```graphql
query GetASN($asnNumber: Int!) {
  asn(number: $asnNumber) {
    number
    name
    validationState
  }
}
```

With variables:
```json
{
  "asnNumber": 3333
}
```

### Error Handling

The API returns errors in a structured format:

```json
{
  "errors": [
    {
      "message": "ASN not found",
      "path": ["asn"],
      "extensions": {
        "code": "NOT_FOUND"
      }
    }
  ]
}
```

Common errors:
- Invalid ASN numbers
- Malformed CIDR prefixes
- Network/database connection issues

### Tools and Resources

#### GraphQL Playground

Access at `http://localhost:8080/playground` to:
- Explore the schema
- Write and test queries
- See documentation for each field
- View query history

#### Schema Introspection

Query the schema itself:

```graphql
query {
  __schema {
    types {
      name
      description
    }
  }
}
```

#### Learning Resources

- [GraphQL Official Documentation](https://graphql.org/learn/)
- [Apollo GraphQL Tutorial](https://www.apollographql.com/docs/)
- [How to GraphQL](https://www.howtographql.com/)

### Troubleshooting

#### Common Issues

1. **"Field X not found"**: Check field names are spelled correctly
2. **"Type Y not found"**: Verify you're using the correct type names
3. **Empty results**: Check your filters and pagination
4. **Slow responses**: Use pagination and request fewer fields

#### Getting Help

1. Use the GraphQL Playground to test queries
2. Check the schema documentation in the Playground
3. Review the examples in this guide
4. Look at the frontend code for working query examples

Remember: GraphQL is forgiving - you can experiment in the Playground without breaking anything!

## Configuration

Environment variables:

- `SERVER_ADDR`: HTTP server address (default: `:8080`)
- `DATABASE_URL`: PostgreSQL connection string
- `REDIS_URL`: Redis connection string
- `INGESTION_INTERVAL`: Data ingestion interval (default: `15m`)
- `LOG_LEVEL`: Logging level (default: `info`)

## Data Sources

The platform ingests data from:

1. **RIPE NCC RPKI**
   - RRDP: `https://rrdp.ripe.net/notification.xml`
   - rsync: `rsync://rpki.ripe.net/repository/`

2. **Cloudflare RPKI**
   - JSON Feed: `https://rpki.cloudflare.com/rpki.json`

3. **NLnet Labs Routinator**
   - HTTP API: `http://localhost:9556/json`

## Development

### Code Generation

Generate GraphQL code:
```bash
go run github.com/99designs/gqlgen generate
```

### Testing

Run tests:
```bash
go test ./...
```

### Database Schema

Apply migrations:
```bash
psql -d rpki_viz -f db/migrations/001_create_tables.sql
```

## Deployment

### Production Docker

```bash
docker build -t rpki-viz .
docker run -p 8080:8080 \
  -e DATABASE_URL="postgres://..." \
  -e REDIS_URL="redis://..." \
  rpki-viz
```

### Kubernetes

See `k8s/` directory for Kubernetes manifests (future implementation).

## Monitoring

- Health check endpoint: `GET /health`
- TODO: Prometheus metrics endpoint
- TODO: Structured logging with OpenTelemetry

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License - see LICENSE file for details.

## References

- [RPKI RFCs](https://tools.ietf.org/html/rfc6480)
- [RIPE NCC RPKI Documentation](https://www.ripe.net/manage-ips-and-asns/resource-management/rpki/)
- [Cloudflare RPKI Tools](https://github.com/cloudflare/cfrpki)
- [NLnet Labs Routinator](https://routinator.docs.nlnetlabs.nl/)