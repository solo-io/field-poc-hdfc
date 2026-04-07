# UI build stage - use BUILDPLATFORM to avoid QEMU emulation issues with esbuild
FROM --platform=$BUILDPLATFORM oven/bun:1-alpine AS ui-builder

WORKDIR /ui

# Copy package files
COPY ui/package.json ui/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy source code
COPY ui/ ./

# Build the UI
RUN bun run build

# Go build stage
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETARCH

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary for target architecture
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s" \
    -o /quota-management \
    ./cmd/server

# Runtime stage
FROM alpine:3.23

# Install CA certificates for HTTPS
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -u 1000 appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /quota-management /app/quota-management

# Copy migrations
COPY --from=builder /app/migrations /app/migrations

# Copy UI static files
COPY --from=ui-builder /ui/dist /app/ui

# Use non-root user
USER appuser

# Expose ports (gRPC, HTTP, Metrics)
EXPOSE 4444 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

# Run the server
ENTRYPOINT ["/app/quota-management"]
