# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o rpki-viz ./cmd/rpki-viz

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests and rsync for RPKI data fetching
RUN apk --no-cache add ca-certificates rsync

# Create app directory and data directory for RPKI data
RUN mkdir -p /app /data/rpki

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/rpki-viz .

# Copy database migrations
COPY --from=builder /app/db/migrations ./db/migrations

# Expose ports
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set default environment variables
ENV SERVER_ADDR=:8080 \
    DATABASE_URL=postgres://user:password@db:5432/rpki_viz?sslmode=disable \
    REDIS_URL=redis://redis:6379 \
    INGESTION_INTERVAL=15m \
    LOG_LEVEL=info

# Run the application
CMD ["./rpki-viz"]