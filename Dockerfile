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

# Build the main application (Kafka disabled temporarily)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Build pdfworker binary for map-reduce PDF processing
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o pdfworker ./cmd/pdfworker

# Build OCR worker binary for Kafka pipeline
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ocrworker ./cmd/ocrworker

# Build assembler binary for Kafka pipeline
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o assembler ./cmd/assembler

# Build DLP worker binary for Kafka pipeline
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o dlpworker ./cmd/dlpworker

# Build embedding worker binary for Kafka pipeline
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o embeddingworker ./cmd/embeddingworker

# Build backfill utility for one-time DLP job publishing
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o backfill-dlp ./cmd/backfill-dlp

# Production stage
FROM alpine:3.18

# Install runtime dependencies including su-exec for privilege dropping
# PDF and OCR tools for text extraction
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    su-exec \
    # PDF tools (pdftotext, pdfimages, pdftoppm)
    poppler-utils \
    # OCR engine
    tesseract-ocr \
    tesseract-ocr-data-eng \
    # Image processing
    imagemagick

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binaries from builder stage
COPY --from=builder /app/main .
COPY --from=builder /app/pdfworker .
COPY --from=builder /app/ocrworker .
COPY --from=builder /app/assembler .
COPY --from=builder /app/dlpworker .
COPY --from=builder /app/embeddingworker .
COPY --from=builder /app/backfill-dlp .

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Note: Configuration files loaded from environment variables

# Create necessary directories
RUN mkdir -p /app/data /app/logs /app/temp && \
    chown -R appuser:appgroup /app

# Set PDF worker path for map-reduce subprocess isolation
ENV PDF_WORKER_PATH=/app/pdfworker

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