# Multi-stage build for eAIIngest platform
FROM golang:1.24-alpine AS builder

# Install build dependencies including C compiler for CGO
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application (Kafka disabled temporarily)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Production stage
FROM alpine:3.18

# Install runtime dependencies including su-exec for privilege dropping
RUN apk --no-cache add ca-certificates tzdata su-exec

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/main .

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Note: Configuration files loaded from environment variables

# Create necessary directories
RUN mkdir -p /app/data /app/logs /app/temp && \
    chown -R appuser:appgroup /app

# NOTE: We do NOT switch to appuser here - the entrypoint script handles that

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set entrypoint to our script
ENTRYPOINT ["/entrypoint.sh"]

# Run the application
CMD ["./main"]