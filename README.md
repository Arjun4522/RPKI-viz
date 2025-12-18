# RPKI-viz

A production-ready RPKI (Resource Public Key Infrastructure) visualization and validation backend that integrates with Routinator for real-time VRP (Validated ROA Payload) data processing and monitoring.

## Architecture Overview

RPKI-viz is a Docker-based microservices architecture consisting of two core components:

1. **Routinator** - Official NLnet Labs RPKI validator daemon
2. **RPKI Backend** - Custom Python application for processing and API exposure

### Component Diagram

```
┌─────────────────┐    HTTP/JSON    ┌─────────────────┐    REST API     ┌─────────────────┐
│   Routinator    │ ◄─────────────── │ RPKI Backend    │ ──────────────► │   Client Apps   │
│ (nlnetlabs/img) │     port 8323   │ (Python/Flask)  │    port 8080   │ (Visualization)  │
└─────────────────┘                 └─────────────────┘                └─────────────────┘
        │                                   │                                    │
        ▼                                   ▼                                    ▼
┌─────────────────┐             ┌─────────────────┐                 ┌─────────────────┐
│ RIR Repositories │             │  State Storage   │                 │   Prometheus    │
│   (Internet)     │             │  (JSON files)    │                 │    Metrics      │
└─────────────────┘             └─────────────────┘                 └─────────────────┘
```

## Features

- **Real-time VRP Processing**: Polls Routinator every 10 minutes for updated RPKI data
- **Change Detection**: Computes diffs between VRP snapshots with monotonic serial numbers
- **RESTful API**: Comprehensive HTTP endpoints for VRP data access and validation
- **Data Validation**: Strict schema validation using Pydantic models
- **Observability**: Prometheus metrics for monitoring and alerting
- **Persistence**: Disk-based state storage with automatic recovery
- **Containerized**: Docker-based deployment with health checks

## API Endpoints

### Core Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Application health status |
| `/metrics` | GET | Prometheus metrics |
| `/api/v1/state` | GET | Current RPKI state metadata |
| `/api/v1/vrps` | GET | Get all VRPs with optional filtering |
| `/api/v1/diff` | GET | Get diff between serial numbers |
| `/api/v1/validate` | POST | Validate BGP route announcement |


### Example Usage

```bash
# Health check
curl http://localhost:8080/health

# Get current state
curl http://localhost:8080/api/v1/state

# Get all VRPs
curl http://localhost:8080/api/v1/vrps

# Filter VRPs by ASN
curl "http://localhost:8080/api/v1/vrps?asn=AS12345"

# Validate a route
curl -X POST http://localhost:8080/api/v1/validate \
  -H "Content-Type: application/json" \
  -d '{"asn": "AS12345", "prefix": "192.0.2.0/24"}'
```

## Data Model

### VRP Entry Schema

```json
{
  "asn": "AS65000",
  "prefix": "192.0.2.0/24",
  "maxLength": 24,
  "ta": "arin"
}
```

### State Response

```json
{
  "serial": 42,
  "vrp_count": 786005,
  "hash": "abc123def456...",
  "last_update": "2023-12-19T01:23:45.678901",
  "vrps": [...]
}
```

## Deployment

### Prerequisites

- Docker Engine 20.10+
- Docker Compose 2.0+
- 2GB RAM minimum
- 10GB disk space for RPKI cache

### Quick Start

1. **Clone the repository**
   ```bash
   git clone git@github.com:Arjun4522/RPKI-viz.git
   cd RPKI-viz
   ```

2. **Start the services**
   ```bash
   docker-compose up -d
   ```

3. **Verify deployment**
   ```bash
   docker-compose ps
   watch -n 10 'curl -s http://localhost:8323/json 2>&1 | head -5'   
   ```
Once Initial Validation complete, restart backend container

### Configuration

Environment variables for the backend service:

| Variable | Default | Description |
|----------|---------|-------------|
| `ROUTINATOR_URL` | `http://routinator:8323` | Routinator JSON endpoint |
| `POLL_INTERVAL_SECONDS` | `600` | Polling interval (10 minutes) |
| `STATE_DIR` | `/app/state` | State storage directory |
| `LOG_LEVEL` | `INFO` | Logging level |
| `API_PORT` | `8080` | API server port |

### Volumes

The system uses Docker volumes for persistent storage:

- `routinator-cache`: RPKI repository cache (~5-10GB)
- `backend-state`: Application state and diffs (~1-2GB)

## Monitoring & Metrics

### Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `rpki_fetch_failures_total` | Counter | Failed VRP fetches |
| `rpki_last_successful_fetch_timestamp` | Gauge | Last successful fetch time |
| `rpki_vrp_count` | Gauge | Current number of VRPs |
| `rpki_serial_number` | Gauge | Current serial number |
| `rpki_snapshot_age_seconds` | Gauge | Age of current snapshot |
| `rpki_api_requests_total` | Counter | API request counts |
| `rpki_api_request_duration_seconds` | Histogram | API request latency |

### Health Checks

- **Routinator**: File existence check in cache directory
- **Backend**: HTTP health endpoint (`/health`)

## Development

### Project Structure

```
RPKI-viz-3/
├── backend/
│   ├── main.py              # Main application controller
│   ├── api_server.py        # Flask API server
│   ├── vrp_loader.py        # Routinator integration
│   ├── diff_engine.py       # Change detection engine
│   ├── metrics.py           # Prometheus metrics
│   ├── requirements.txt     # Python dependencies
│   └── Dockerfile          # Backend container definition
├── docker-compose.yml       # Multi-container deployment
└── README.md               # This file
```

### Building from Source

1. **Build the backend image**
   ```bash
   cd backend
   docker build -t rpki-backend .
   ```

2. **Update docker-compose.yml**
   ```yaml
   rpki-backend:
     image: rpki-backend:latest
     # instead of build: ./backend
   ```

### Testing

```bash
# Test API endpoints
curl http://localhost:8080/health
curl http://localhost:8080/metrics

# Test data processing
curl http://localhost:8080/api/v1/state
curl http://localhost:8080/api/v1/vrps?asn=AS15169

# Test route validation
curl -X POST http://localhost:8080/api/v1/validate \
  -H "Content-Type: application/json" \
  -d '{"asn": "AS15169", "prefix": "8.8.8.0/24"}'
```

## Performance Characteristics

- **VRP Processing**: ~786,000 VRPs processed in ~60 seconds
- **Memory Usage**: Backend ~256MB, Routinator ~512MB
- **Storage**: ~1GB for state, ~5GB for RPKI cache
- **API Response Time**: <100ms for most endpoints

## Troubleshooting

### Common Issues

1. **Routinator not ready**
   ```bash
   # Check Routinator logs
   docker logs routinator
   
   # Wait for initial sync (5-10 minutes)
   ```

2. **Backend unhealthy**
   ```bash
   # Healthcheck uses wget which may not be available
   # The API is still functional - check /health endpoint
   curl http://localhost:8080/health
   ```

3. **VRP fetch failures**
   ```bash
   # Check Routinator accessibility
   curl http://localhost:8323/json | head -n 5
   ```

### Logs

```bash
# View backend logs
docker logs rpki-backend

# View Routinator logs
docker logs routinator

# Follow logs in real-time
docker logs -f rpki-backend
```

## Security Considerations

- Non-root user execution in containers
- Read-only access to RPKI data
- No authentication on API endpoints (intended for internal use)
- Input validation on all API parameters
- No persistent secrets or credentials

## Scaling Considerations

- **Horizontal Scaling**: Multiple backend instances can read from shared state
- **Load Balancing**: API endpoints are stateless and cache-friendly
- **Storage**: State directory can be mounted from network storage
- **Monitoring**: All metrics exposed for Prometheus scraping

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Submit a pull request

## License

[Add appropriate license information]

## References

- [RPKI Overview](https://rpki.readthedocs.io/en/latest/)
- [Routinator Documentation](https://routinator.docs.nlnetlabs.nl/)
- [RFC 6480 - RPKI Architecture](https://tools.ietf.org/html/rfc6480)
- [RFC 6811 - BGP Prefix Origin Validation](https://tools.ietf.org/html/rfc6811)

## Support

For issues and questions:
1. Check the troubleshooting section
2. Review container logs
3. Examine API responses
4. Open a GitHub issue