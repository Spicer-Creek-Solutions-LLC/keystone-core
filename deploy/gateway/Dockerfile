# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
# hadolint ignore=DL3018
RUN apk add --no-cache git

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the gateway binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /kscore-telemetry-gateway ./cmd/kscore-telemetry-gateway

# Runtime stage
FROM alpine:3.19

# Add ca-certificates for TLS connections and create non-root user
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -g '' kscore
USER kscore

# Copy binary from builder
COPY --from=builder /kscore-telemetry-gateway /usr/local/bin/

# Copy example config
COPY --from=builder /app/deploy/gateway/config.yaml.example /etc/keystone-core/config.yaml.example

# Expose default port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Run the gateway
ENTRYPOINT ["kscore-telemetry-gateway"]
CMD ["--config", "/etc/keystone-core/config.yaml"]
