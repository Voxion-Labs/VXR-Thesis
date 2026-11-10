# ==============================================================================
# VXR-Thesis: Production Multi-Stage Dockerfile
# Optimized for 512MB Edge Execution Profile
# ==============================================================================

# Stage 1: Build the static Go binary
FROM golang:1.21-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -extldflags '-static'" \
    -o /bin/vxr-server \
    ./cmd/vxr-server

# Stage 2: Minimalist, hardened runtime container
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S vxr && adduser -S vxr -G vxr

# Copy compiled binary from builder
COPY --from=builder /bin/vxr-server /app/vxr-server

# Copy web templates and static assets
COPY web/ /app/web/

# Create telemetry data directory with non-root ownership
RUN mkdir -p /app/benchmarks/data && chown -R vxr:vxr /app

USER vxr

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/metrics/json || exit 1

ENTRYPOINT ["/app/vxr-server"]
CMD ["-serve", "-port", "8080", "-wasm", "web/static/wasm/vxr_kernel.wasm", "-csv", "benchmarks/data/telemetry.csv"]
