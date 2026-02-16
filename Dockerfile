# Multi-stage build for kueue-bench
# Stage 1: Build the binary
FROM golang:1.26-alpine AS builder

WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build the binary
# CGO_ENABLED=0 for static binary
# Trim path for smaller binary size
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o kueue-bench \
    ./cmd/kueue-bench

# Stage 2: Runtime image
FROM alpine:3.19

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /workspace/kueue-bench /usr/local/bin/kueue-bench

# Create directory for topology metadata
RUN mkdir -p /root/.kueue-bench

ENTRYPOINT ["kueue-bench"]
CMD ["--help"]
