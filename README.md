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

## API Usage

### GraphQL Queries

Get ASN information:
```graphql
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

Validate a prefix:
```graphql
query {
  validatePrefix(asn: 3333, prefix: "193.0.0.0/21") {
    asn
    prefix
    state
    reason
  }
}
```

Get global summary:
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