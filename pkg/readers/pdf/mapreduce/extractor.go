package mapreduce

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DefaultPageExtractor implements PageExtractor
type DefaultPageExtractor struct {
	config *ExtractionConfig
}

// NewPageExtractor creates a new page extractor
func NewPageExtractor(config *ExtractionConfig) *DefaultPageExtractor {
	if config == nil {
		config = DefaultExtractionConfig()
	}
	return &DefaultPageExtractor{
		config: config,
	}
}

// ExtractPage extracts content from a single PDF page
func (e *DefaultPageExtractor) ExtractPage(ctx context.Context, job *PageJob) (*PageResult, error) {
	startTime := time.Now()

	result := &PageResult{
		JobID:          job.JobID,
		TenantID:       job.TenantID,
		FileID:         job.FileID,
		PageNumber:     job.PageNumber,
		TotalPages:     job.TotalPages,
		Classification: job.Classification,
		Status:         StatusCompleted,
		ProcessedAt:    time.Now(),
		WorkerID:       getWorkerID(),
	}

	// Choose extraction method based on classification
	var content string
	var method ExtractionMethod
	var confidence float64
	var err error

	switch job.Classification {
	case ClassTextOnly:
		content, err = e.extractWithPDFToText(ctx, job)
		method = MethodPDFToText
		confidence = 1.0

	case ClassScanned, ClassImageOnly:
		content, confidence, err = e.extractWithOCR(ctx, job)
		method = MethodOCR

	case ClassHybrid:
		content, confidence, err = e.extractHybrid(ctx, job)
		method = MethodHybrid

	case ClassEmpty:
		content = ""
		method = MethodSkipped
		confidence = 1.0
		result.Status = StatusSkipped

	default:
		// Unknown classification - try OCR as fallback
		content, confidence, err = e.extractWithOCR(ctx, job)
		method = MethodOCR
	}

	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		result.ProcessingDurationMs = time.Since(startTime).Milliseconds()
		return result, nil // Return result with error info, don't propagate error
	}

	result.Content = content
	result.ExtractionMethod = method
	result.OCRConfidence = confidence
	result.ContentHash = hashContent(content)
	result.ProcessingDurationMs = time.Since(startTime).Milliseconds()

	// Set OCR params if OCR was used
	if method == MethodOCR || method == MethodHybrid {
		config := e.getConfig(job)
		result.OCRLanguage = config.OCRLanguage
		result.OCRDPI = config.OCRDPI
	}

	return result, nil
}

// extractWithPDFToText extracts text using pdftotext (fast path)
func (e *DefaultPageExtractor) extractWithPDFToText(ctx context.Context, job *PageJob) (string, error) {
	config := e.getConfig(job)

	args := []string{
		"-f", strconv.Itoa(job.PageNumber),
		"-l", strconv.Itoa(job.PageNumber),
	}

	if config.PreserveLayout {
		args = append(args, "-layout")
	}

	args = append(args, job.PDFPath, "-") // Output to stdout

	cmd := exec.CommandContext(ctx, "pdftotext", args...)

	// Use streaming pipe
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("pdftotext failed to start: %w", err)
	}

	// Stream output
	var result strings.Builder
	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 64KB initial, 1MB max

	for scanner.Scan() {
		result.WriteString(scanner.Text())
		result.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		cmd.Wait()
		return "", fmt.Errorf("error reading pdftotext output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("pdftotext failed: %w (stderr: %s)", err, stderr.String())
	}

	return strings.TrimSpace(result.String()), nil
}

// extractWithOCR extracts text using OCR (tesseract)
func (e *DefaultPageExtractor) extractWithOCR(ctx context.Context, job *PageJob) (string, float64, error) {
	config := e.getConfig(job)

	// Create temp directory for this page
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("pdf-ocr-page-%d-", job.PageNumber))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Step 1: Render page to image using pdftoppm
	imgPath, err := e.renderPageToImage(ctx, job.PDFPath, job.PageNumber, tmpDir, config.OCRDPI)
	if err != nil {
		return "", 0, fmt.Errorf("failed to render page: %w", err)
	}

	// Step 2: OCR the image
	text, confidence, err := e.ocrImage(ctx, imgPath, config.OCRLanguage)
	if err != nil {
		return "", 0, fmt.Errorf("OCR failed: %w", err)
	}

	// Force garbage collection to free image memory
	runtime.GC()

	return text, confidence, nil
}

// extractHybrid extracts using both pdftotext and OCR
func (e *DefaultPageExtractor) extractHybrid(ctx context.Context, job *PageJob) (string, float64, error) {
	// First try pdftotext
	textContent, err := e.extractWithPDFToText(ctx, job)
	if err != nil {
		// Fall back to pure OCR
		return e.extractWithOCR(ctx, job)
	}

	// If we got reasonable text, return it
	if len(textContent) > 100 {
		return textContent, 1.0, nil
	}

	// Otherwise, supplement with OCR
	ocrContent, confidence, err := e.extractWithOCR(ctx, job)
	if err != nil {
		// Return whatever text we got from pdftotext
		return textContent, 0.5, nil
	}

	// Combine results
	if len(ocrContent) > len(textContent) {
		return ocrContent, confidence, nil
	}

	return textContent, 1.0, nil
}

// renderPageToImage renders a PDF page to PNG using pdftoppm
func (e *DefaultPageExtractor) renderPageToImage(ctx context.Context, pdfPath string, pageNum int, tmpDir string, dpi int) (string, error) {
	imgPrefix := filepath.Join(tmpDir, "page")

	cmd := exec.CommandContext(ctx, "pdftoppm",
		"-f", strconv.Itoa(pageNum),
		"-l", strconv.Itoa(pageNum),
		"-r", strconv.Itoa(dpi),
		"-png",
		"-singlefile",
		pdfPath,
		imgPrefix)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftoppm failed: %w (stderr: %s)", err, stderr.String())
	}

	imgPath := imgPrefix + ".png"
	if _, err := os.Stat(imgPath); err != nil {
		return "", fmt.Errorf("rendered image not found: %w", err)
	}

	return imgPath, nil
}

// preprocessImage applies gentle denoising to improve OCR quality without
// destroying character shapes. Tesseract handles binarization internally.
// Requires ImageMagick's convert command.
func (e *DefaultPageExtractor) preprocessImage(ctx context.Context, imgPath string) (string, error) {
	dir := filepath.Dir(imgPath)
	ext := filepath.Ext(imgPath)
	base := strings.TrimSuffix(filepath.Base(imgPath), ext)
	outPath := filepath.Join(dir, base+"_preprocessed"+ext)

	// Gentle preprocessing: grayscale + normalize contrast + light denoise
	// Avoid binarization — Tesseract's internal Otsu threshold is better
	cmd := exec.CommandContext(ctx, "convert", imgPath,
		"-colorspace", "Gray",
		"-normalize",   // Stretch contrast to full range
		"-despeckle",   // Light denoise
		outPath)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Preprocessing is best-effort; fall back to original
		return imgPath, nil
	}

	return outPath, nil
}

// ocrImage runs tesseract on an image file with quality-optimized settings
func (e *DefaultPageExtractor) ocrImage(ctx context.Context, imgPath string, language string) (string, float64, error) {
	// Preprocess image for better OCR quality
	processedPath, _ := e.preprocessImage(ctx, imgPath)

	// Try with TSV output for confidence
	text, confidence, err := e.ocrImageWithTSV(ctx, processedPath, language)
	if err == nil {
		return text, confidence, nil
	}

	// Fall back to plain text output
	cmd := exec.CommandContext(ctx, "tesseract", processedPath, "stdout",
		"-l", language,
		"--oem", "1",  // LSTM neural net engine (best quality)
		"--psm", "3")  // Fully automatic page segmentation

	output, err := cmd.Output()
	if err != nil {
		return "", 0, fmt.Errorf("tesseract failed: %w", err)
	}

	return strings.TrimSpace(string(output)), 0.8, nil // Default confidence
}

// ocrImageWithTSV runs tesseract with TSV output for confidence scores
func (e *DefaultPageExtractor) ocrImageWithTSV(ctx context.Context, imgPath string, language string) (string, float64, error) {
	cmd := exec.CommandContext(ctx, "tesseract", imgPath, "stdout",
		"-l", language,
		"--oem", "1",  // LSTM neural net engine (best quality)
		"--psm", "3",  // Fully automatic page segmentation
		"tsv")

	output, err := cmd.Output()
	if err != nil {
		return "", 0, err
	}

	return parseTesseractTSV(string(output))
}

// parseTesseractTSV parses tesseract TSV output to extract text and confidence
func parseTesseractTSV(tsv string) (string, float64, error) {
	lines := strings.Split(tsv, "\n")
	if len(lines) < 2 {
		return "", 0, fmt.Errorf("invalid TSV output")
	}

	var textParts []string
	var totalConf float64
	var wordCount int

	for i, line := range lines {
		if i == 0 {
			continue // Skip header
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue
		}

		// TSV format: level, page_num, block_num, par_num, line_num, word_num,
		//             left, top, width, height, conf, text
		text := strings.TrimSpace(fields[11])
		if text == "" {
			continue
		}

		textParts = append(textParts, text)

		conf, err := strconv.ParseFloat(fields[10], 64)
		if err == nil && conf >= 0 {
			totalConf += conf
			wordCount++
		}
	}

	if wordCount == 0 {
		return strings.Join(textParts, " "), 0.5, nil
	}

	avgConf := totalConf / float64(wordCount) / 100.0 // Convert from 0-100 to 0-1
	return strings.Join(textParts, " "), avgConf, nil
}

// getConfig returns the extraction config, merging job config with defaults
func (e *DefaultPageExtractor) getConfig(job *PageJob) *ExtractionConfig {
	if job.Config != nil {
		return job.Config
	}
	return e.config
}

// getWorkerID returns a unique identifier for this worker
func getWorkerID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

// hashContent generates an MD5 hash of the content
func hashContent(content string) string {
	hash := md5.Sum([]byte(content))
	return hex.EncodeToString(hash[:])
}

// ExtractSinglePage is a convenience function for extracting a single page
func ExtractSinglePage(ctx context.Context, pdfPath string, pageNum int, classification PageClassificationType, config *ExtractionConfig) (*PageResult, error) {
	extractor := NewPageExtractor(config)

	job := &PageJob{
		JobID:          uuid.New().String(),
		TenantID:       uuid.Nil,
		FileID:         uuid.Nil,
		PDFPath:        pdfPath,
		PageNumber:     pageNum,
		TotalPages:     1,
		Classification: classification,
		Config:         config,
	}

	return extractor.ExtractPage(ctx, job)
}
