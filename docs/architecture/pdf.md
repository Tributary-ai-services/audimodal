# PDF Processing Architecture

This document describes AudiModal's PDF processing architecture, including the map-reduce pipeline for large document handling.

---

## Overview

AudiModal supports two PDF processing modes:

| Mode | Page Threshold | Use Case |
|------|----------------|----------|
| **Streaming** | < 50 pages | Small documents, low memory |
| **Map-Reduce** | >= 50 pages | Large documents, memory isolation |

The mode is auto-selected based on page count, or can be explicitly configured.

---

## Map-Reduce Architecture

For large PDFs (512+ pages), in-process OCR causes memory accumulation leading to OOM errors. The map-reduce architecture solves this by:

1. **Page Classification** - Determine extraction method per page
2. **Subprocess Isolation** - Run OCR in separate processes that exit after each page
3. **Intermediate Storage** - Save each page's output before aggregation
4. **Reduce Phase** - Combine page results into document chunks

### Architecture Diagram

```
PDF File
    |
    v
+-------------------------------------------------------------+
|  PHASE 1: MAP - Page Classification                         |
|  +-----+ +-----+ +-----+     +-----+                        |
|  | P1  | | P2  | | P3  | ... | PN  |  (parallel)            |
|  +--+--+ +--+--+ +--+--+     +--+--+                        |
|     |       |       |           |                           |
|  classify classify classify  classify                       |
|     |       |       |           |                           |
|     v       v       v           v                           |
|  text_only scanned hybrid    text_only                      |
+-------------------------------------------------------------+
    |
    v
+-------------------------------------------------------------+
|  PHASE 2: MAP - Extraction (subprocess isolation)           |
|  +---------+  +---------+  +---------+                      |
|  | Worker1 |  | Worker2 |  | Worker3 |  (worker pool)       |
|  | pdftext |  | OCR+exit|  | OCR+exit|                      |
|  +----+----+  +----+----+  +----+----+                      |
|       |            |            |                           |
|       v            v            v                           |
|    [Save to DB immediately - PageResult table]              |
+-------------------------------------------------------------+
    |
    v
+-------------------------------------------------------------+
|  PHASE 3: REDUCE - Aggregation                              |
|  - Fetch completed PageResults ordered by page_number       |
|  - Combine text respecting page boundaries                  |
|  - Apply chunking strategy                                  |
|  - Continue existing embedding/DLP pipeline                 |
+-------------------------------------------------------------+
```

---

## Page Classification

Each page is classified to determine the optimal extraction method:

| Classification | Description | Extraction Method |
|----------------|-------------|-------------------|
| `text_only` | Has embedded text (>100 chars) | pdftotext (fast) |
| `scanned` | No fonts, large image | OCR (tesseract) |
| `hybrid` | Text + significant images | pdftotext + OCR |
| `image_only` | Images with minimal text | OCR |
| `empty` | No content | Skipped |

### Classification Algorithm

```
1. Quick pdftotext extraction
   - If >100 chars -> text_only (confidence: 0.95)

2. Check pdffonts for embedded fonts
   - No fonts + large image -> scanned (confidence: 0.90)

3. Check pdfimages for large images (>50% page)
   - Text + images -> hybrid (confidence: 0.85)

4. Fallback
   - Low text, has images -> image_only (confidence: 0.85)
   - Very low text, no images -> empty (confidence: 0.95)
```

---

## Worker Pool

The worker pool manages concurrent page processing with memory isolation:

### DefaultWorkerPool (Subprocess)

- Spawns `pdfworker` binary for each page
- Worker receives job via stdin (JSON)
- Worker outputs result to stdout (JSON)
- Worker exits immediately - OS reclaims memory
- Semaphore limits concurrent workers (default: 4)

### InlineWorkerPool (In-Process)

- Fallback for environments without subprocess support
- Used for testing and debugging
- Same interface as DefaultWorkerPool

### Configuration

```go
type CoordinatorConfig struct {
    MaxConcurrentWorkers   int  // Default: 4
    WorkerTimeoutSeconds   int  // Default: 300
    MaxRetries             int  // Default: 3
    RetryDelay             int  // Default: 5 seconds
    CheckpointInterval     int  // Default: 10 pages
    MapReducePageThreshold int  // Default: 50 pages
}
```

---

## External Tools

The PDF processing pipeline uses these external tools:

| Tool | Package | Purpose |
|------|---------|---------|
| `pdftotext` | poppler-utils | Extract text from PDF pages |
| `pdfinfo` | poppler-utils | Get PDF metadata and page count |
| `pdffonts` | poppler-utils | Detect embedded fonts |
| `pdfimages` | poppler-utils | Detect/extract images |
| `pdftoppm` | poppler-utils | Convert PDF to images for OCR |
| `tesseract` | tesseract-ocr | OCR engine |

### Installation

```bash
# Ubuntu/Debian
sudo apt-get install -y poppler-utils tesseract-ocr tesseract-ocr-eng

# macOS
brew install poppler tesseract

# Or use the installation script
./scripts/install-pdf-tools.sh
```

---

## Package Structure

```
pkg/readers/pdf/
├── reader.go              # Main PDF reader with mode selection
└── mapreduce/
    ├── types.go           # Shared types and interfaces
    ├── classifier.go      # Page classification logic
    ├── pool.go            # Worker pool management
    ├── extractor.go       # Page extraction (text/OCR)
    ├── coordinator.go     # Map-reduce orchestration
    └── reducer.go         # Result aggregation

cmd/pdfworker/
└── main.go                # Standalone worker binary
```

---

## Configuration Options

### Environment Variables

```bash
PDF_PROCESSING_MODE=auto              # auto, streaming, mapreduce
PDF_WORKER_PATH=/path/to/pdfworker    # Path to pdfworker binary (enables subprocess isolation)
PDF_MAPREDUCE_WORKERS=4               # Parallel workers
PDF_MAPREDUCE_WORKER_MEMORY_MB=1024   # Memory limit per worker
PDF_MAPREDUCE_PAGE_TIMEOUT=300        # Seconds per page
PDF_MAPREDUCE_CHECKPOINT_INTERVAL=10  # Save progress every N pages
PDF_OCR_DPI=150                       # Lower = less memory
PDF_TEXT_THRESHOLD=100                # Chars needed for text_only
```

**Important:** To enable subprocess memory isolation (recommended for large PDFs), set `PDF_WORKER_PATH` to the location of the `pdfworker` binary:
```bash
export PDF_WORKER_PATH=/app/bin/pdfworker  # Docker
export PDF_WORKER_PATH=./bin/pdfworker     # Local development
```

### Reader Configuration

```go
config := map[string]any{
    "processing_mode":           "auto",  // auto, streaming, mapreduce
    "mapreduce_page_threshold":  50,      // Pages threshold for mapreduce
    "mapreduce_workers":         4,       // Concurrent workers
    "ocr_language":              "eng",   // Tesseract language
    "ocr_dpi":                   150,     // OCR resolution
    "preserve_layout":           true,    // Preserve text layout
}
```

---

## Memory Optimizations

1. **Subprocess isolation** - OCR worker exits after each page, OS reclaims memory
2. **Page classification** - Skip OCR on text-only pages (saves 80%+ for text PDFs)
3. **Lower DPI** - 150 DPI instead of 300 (4x memory reduction)
4. **Immediate storage** - Save each page result to DB, don't accumulate in memory
5. **Worker pool limits** - Max 4 concurrent workers (configurable)
6. **Checkpoint/recovery** - Resume from last successful page on failure

---

## Testing

### Environment Setup

```bash
# Set test PDF path
export TEST_PDF_PATH=/home/jscharber/eng/TAS/tas-test-data/pdf/TAS_Data_Models_Consolidated.pdf

# Run tests
go test -v ./pkg/readers/pdf/mapreduce/...

# Run benchmarks
go test -bench=. ./pkg/readers/pdf/mapreduce/...
```

### Test Coverage

- `types_test.go` - Configuration defaults, classification types
- `classifier_test.go` - Page classification logic
- `extractor_test.go` - Text extraction, OCR, TSV parsing
- `pool_test.go` - Worker pool, concurrency, shutdown
- `reducer_test.go` - Result aggregation, chunk generation

---

## Database Schema

Page results are stored in the `page_results` table for checkpoint/recovery:

```sql
CREATE TABLE page_results (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    file_id UUID NOT NULL,
    page_number INTEGER NOT NULL,
    total_pages INTEGER NOT NULL,
    classification VARCHAR(50) NOT NULL,
    content TEXT,
    extraction_method VARCHAR(50) NOT NULL,
    processing_status VARCHAR(50) NOT NULL,
    ocr_confidence DECIMAL(5,4),
    processing_time_ms INTEGER,
    retry_count INTEGER DEFAULT 0,
    error_message TEXT,
    worker_id VARCHAR(255),
    content_hash VARCHAR(64),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(file_id, page_number)
);
```

---

## Image Captioning Models (Reference)

For image-heavy PDFs, these models can generate captions for embedded images:

| Model | Best For | Speed |
|-------|----------|-------|
| BLIP | Fast captioning at scale | Fast |
| BLIP-2 | High-quality captions + reasoning | Moderate |
| GIT | Simple one-shot captioning | Moderate |
| LLaVA | Conversational image reasoning | Slow |
| Kosmos-2 | Mixed image/text layouts | Moderate |

### Recommended Pipeline

1. **BLIP-2** for descriptive captions of extracted images
2. **Text embedding model** (OpenAI/BGE) on captions
3. **CLIP** (optional) for raw image embeddings for dual-mode search
