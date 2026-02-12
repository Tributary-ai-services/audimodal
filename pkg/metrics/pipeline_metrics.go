package metrics

// PipelineMetrics contains metrics for the Kafka document processing pipeline.
type PipelineMetrics struct {
	// Splitter metrics
	SplitterPagesQueued   *Counter
	SplitterSplitDuration *Timer

	// OCR Worker metrics
	OCRPagesProcessed  *Counter
	OCRPageDuration    *Timer
	OCRActiveWorkers   *Gauge
	OCRPDFCacheHits    *Counter
	OCRProcessingErrors *Counter

	// Assembler metrics
	AssemblerFilesCompleted   *Counter
	AssemblerChunksStored     *Counter
	AssemblerS3UploadDuration *Timer
	AssemblerStaleJobs        *Counter

	// DLP Worker metrics
	DLPChunksScanned    *Counter
	DLPChunkDuration    *Timer
	DLPFindingsDetected *Counter
	DLPProcessingErrors *Counter

	// Embedding Worker metrics
	EmbeddingChunksProcessed *Counter
	EmbeddingChunkDuration   *Timer
	EmbeddingProcessingErrors *Counter
	EmbeddingFilesCompleted  *Counter

	// Kafka consumer metrics
	KafkaMessagesConsumed  *Counter
	KafkaProcessingErrors  *Counter
}

// NewPipelineMetrics creates and registers all pipeline metrics.
func NewPipelineMetrics(registry *MetricsRegistry) *PipelineMetrics {
	if registry == nil {
		registry = GetRegistry()
	}

	return &PipelineMetrics{
		// Splitter
		SplitterPagesQueued: registry.NewCounter(
			"audimodal_splitter_pages_queued_total",
			"Total pages queued to Kafka by splitter",
			map[string]string{"component": "splitter"},
		),
		SplitterSplitDuration: registry.NewTimer(
			"audimodal_splitter_split_duration_seconds",
			"Time to split a PDF into page jobs",
			map[string]string{"component": "splitter"},
		),

		// OCR Worker
		OCRPagesProcessed: registry.NewCounter(
			"audimodal_ocr_pages_processed_total",
			"Total pages processed by OCR workers",
			map[string]string{"component": "ocr_worker"},
		),
		OCRPageDuration: registry.NewTimer(
			"audimodal_ocr_page_duration_seconds",
			"Time to process a single page via OCR",
			map[string]string{"component": "ocr_worker"},
		),
		OCRActiveWorkers: registry.NewGauge(
			"audimodal_ocr_active_workers",
			"Number of active OCR workers",
			map[string]string{"component": "ocr_worker"},
		),
		OCRPDFCacheHits: registry.NewCounter(
			"audimodal_ocr_pdf_cache_hits_total",
			"PDF download cache hits",
			map[string]string{"component": "ocr_worker"},
		),
		OCRProcessingErrors: registry.NewCounter(
			"audimodal_ocr_processing_errors_total",
			"OCR processing errors",
			map[string]string{"component": "ocr_worker"},
		),

		// Assembler
		AssemblerFilesCompleted: registry.NewCounter(
			"audimodal_assembler_files_completed_total",
			"Total files assembled",
			map[string]string{"component": "assembler"},
		),
		AssemblerChunksStored: registry.NewCounter(
			"audimodal_assembler_chunks_stored_total",
			"Total chunks stored to S3",
			map[string]string{"component": "assembler"},
		),
		AssemblerS3UploadDuration: registry.NewTimer(
			"audimodal_assembler_s3_upload_duration_seconds",
			"Time to upload chunks to S3",
			map[string]string{"component": "assembler"},
		),
		AssemblerStaleJobs: registry.NewCounter(
			"audimodal_assembler_stale_jobs_detected_total",
			"Stale jobs detected and timed out",
			map[string]string{"component": "assembler"},
		),

		// DLP Worker
		DLPChunksScanned: registry.NewCounter(
			"audimodal_dlp_chunks_scanned_total",
			"Total chunks scanned by DLP workers",
			map[string]string{"component": "dlp_worker"},
		),
		DLPChunkDuration: registry.NewTimer(
			"audimodal_dlp_chunk_duration_seconds",
			"Time to DLP-scan a single chunk",
			map[string]string{"component": "dlp_worker"},
		),
		DLPFindingsDetected: registry.NewCounter(
			"audimodal_dlp_findings_detected_total",
			"Total DLP findings detected",
			map[string]string{"component": "dlp_worker"},
		),
		DLPProcessingErrors: registry.NewCounter(
			"audimodal_dlp_processing_errors_total",
			"DLP processing errors",
			map[string]string{"component": "dlp_worker"},
		),

		// Embedding Worker
		EmbeddingChunksProcessed: registry.NewCounter(
			"audimodal_embedding_chunks_processed_total",
			"Total chunks embedded by embedding workers",
			map[string]string{"component": "embedding_worker"},
		),
		EmbeddingChunkDuration: registry.NewTimer(
			"audimodal_embedding_chunk_duration_seconds",
			"Time to embed a single chunk",
			map[string]string{"component": "embedding_worker"},
		),
		EmbeddingProcessingErrors: registry.NewCounter(
			"audimodal_embedding_processing_errors_total",
			"Embedding processing errors",
			map[string]string{"component": "embedding_worker"},
		),
		EmbeddingFilesCompleted: registry.NewCounter(
			"audimodal_embedding_files_completed_total",
			"Files fully embedded (all chunks done)",
			map[string]string{"component": "embedding_worker"},
		),

		// Kafka
		KafkaMessagesConsumed: registry.NewCounter(
			"audimodal_kafka_messages_consumed_total",
			"Total Kafka messages consumed",
			map[string]string{"component": "kafka"},
		),
		KafkaProcessingErrors: registry.NewCounter(
			"audimodal_kafka_processing_errors_total",
			"Kafka message processing errors",
			map[string]string{"component": "kafka"},
		),
	}
}
