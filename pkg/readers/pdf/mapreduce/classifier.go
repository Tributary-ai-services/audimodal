package mapreduce

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// DefaultPageClassifier implements PageClassifier using poppler tools
type DefaultPageClassifier struct {
	config *ExtractionConfig
}

// NewPageClassifier creates a new page classifier
func NewPageClassifier(config *ExtractionConfig) *DefaultPageClassifier {
	if config == nil {
		config = DefaultExtractionConfig()
	}
	return &DefaultPageClassifier{
		config: config,
	}
}

// ClassifyPage determines the type of content on a PDF page
func (c *DefaultPageClassifier) ClassifyPage(ctx context.Context, pdfPath string, pageNum int) (*PageClassification, error) {
	result := &PageClassification{
		PageNumber:     pageNum,
		Classification: ClassUnknown,
		Confidence:     0.5,
	}

	// Step 1: Quick pdftotext extraction to check for extractable text
	textCharCount, err := c.quickTextExtract(ctx, pdfPath, pageNum)
	if err != nil {
		// Log but don't fail - we can still check other signals
		textCharCount = 0
	}
	result.TextCharCount = textCharCount

	// Step 2: Check for fonts on this page using pdffonts
	hasFonts, err := c.checkFonts(ctx, pdfPath, pageNum)
	if err != nil {
		// Log but don't fail
		hasFonts = false
	}
	result.HasEmbeddedFonts = hasFonts

	// Step 3: Check for images using pdfimages -list
	imageInfo, err := c.checkImages(ctx, pdfPath, pageNum)
	if err != nil {
		// Log but don't fail
		imageInfo = &imageCheckResult{}
	}
	result.HasImages = imageInfo.count > 0
	result.ImageCount = imageInfo.count
	result.LargeImageCount = imageInfo.largeCount

	// Classification logic based on gathered signals
	result.Classification, result.Confidence = c.classify(result)

	return result, nil
}

// ClassifyAllPages classifies all pages in parallel
func (c *DefaultPageClassifier) ClassifyAllPages(ctx context.Context, pdfPath string, totalPages int) ([]*PageClassification, error) {
	results := make([]*PageClassification, totalPages)
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make([]error, 0)

	// Use semaphore to limit concurrent classification
	// Classification is lightweight (pdftotext + pdffonts + pdfimages -list)
	sem := make(chan struct{}, 8) // Max 8 concurrent classifications

	for i := 1; i <= totalPages; i++ {
		wg.Add(1)
		pageNum := i

		go func() {
			defer wg.Done()

			// Check context before starting
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
				defer func() { <-sem }()
			}

			classification, err := c.ClassifyPage(ctx, pdfPath, pageNum)
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errs = append(errs, fmt.Errorf("page %d: %w", pageNum, err))
				// Create a default classification for failed pages
				results[pageNum-1] = &PageClassification{
					PageNumber:     pageNum,
					Classification: ClassUnknown,
					Confidence:     0.0,
				}
			} else {
				results[pageNum-1] = classification
			}
		}()
	}

	wg.Wait()

	// Check if all classifications failed
	allFailed := true
	for _, r := range results {
		if r != nil && r.Classification != ClassUnknown {
			allFailed = false
			break
		}
	}

	if allFailed && len(errs) > 0 {
		return nil, fmt.Errorf("all page classifications failed: %v", errs[0])
	}

	return results, nil
}

// quickTextExtract does a fast text extraction to count characters
func (c *DefaultPageClassifier) quickTextExtract(ctx context.Context, pdfPath string, pageNum int) (int, error) {
	cmd := exec.CommandContext(ctx, "pdftotext",
		"-f", strconv.Itoa(pageNum),
		"-l", strconv.Itoa(pageNum),
		"-q", // Quiet mode
		pdfPath,
		"-") // Output to stdout

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("pdftotext failed: %w", err)
	}

	// Count non-whitespace characters
	text := strings.TrimSpace(string(output))
	return len(text), nil
}

// checkFonts checks if the page has embedded fonts using pdffonts
func (c *DefaultPageClassifier) checkFonts(ctx context.Context, pdfPath string, pageNum int) (bool, error) {
	cmd := exec.CommandContext(ctx, "pdffonts",
		"-f", strconv.Itoa(pageNum),
		"-l", strconv.Itoa(pageNum),
		pdfPath)

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("pdffonts failed: %w", err)
	}

	// Parse pdffonts output
	// Format: name                                 type              encoding         emb sub uni object ID
	// If there are fonts, there will be data lines after the header
	lines := strings.Split(string(output), "\n")

	// Skip header lines (typically 2: column names and separator)
	dataLines := 0
	for i, line := range lines {
		// Skip first two header lines
		if i < 2 {
			continue
		}
		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}
		dataLines++
	}

	return dataLines > 0, nil
}

// imageCheckResult contains image analysis results
type imageCheckResult struct {
	count      int
	largeCount int
}

// checkImages checks for images on the page using pdfimages -list
func (c *DefaultPageClassifier) checkImages(ctx context.Context, pdfPath string, pageNum int) (*imageCheckResult, error) {
	cmd := exec.CommandContext(ctx, "pdfimages",
		"-f", strconv.Itoa(pageNum),
		"-l", strconv.Itoa(pageNum),
		"-list",
		pdfPath)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pdfimages failed: %w", err)
	}

	result := &imageCheckResult{}

	// Get configurable thresholds from config, with sensible defaults
	minWidth := c.config.OCRImageMinWidth
	minHeight := c.config.OCRImageMinHeight
	anyImage := c.config.OCRAnyImage

	// Apply defaults if not set
	if minWidth <= 0 {
		minWidth = 200
	}
	if minHeight <= 0 {
		minHeight = 200
	}

	// Parse pdfimages -list output
	// Format: page   num  type   width height color comp bpc  enc interp  object ID x-ppi y-ppi size ratio
	scanner := bufio.NewScanner(bytes.NewReader(output))
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Skip header lines (typically 2)
		if lineNum <= 2 {
			continue
		}

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		result.count++

		// If anyImage is true, count all images as "large" (triggering OCR)
		if anyImage {
			result.largeCount++
			continue
		}

		// Parse width and height to determine if it's a "large" image
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			width, _ := strconv.Atoi(fields[3])
			height, _ := strconv.Atoi(fields[4])

			// Consider an image "large" if it meets the minimum thresholds
			// This indicates an image significant enough to potentially contain text
			if width >= minWidth && height >= minHeight {
				result.largeCount++
			}
		}
	}

	return result, nil
}

// classify determines the classification based on gathered signals
func (c *DefaultPageClassifier) classify(pc *PageClassification) (PageClassificationType, float64) {
	threshold := c.config.TextThreshold

	// If we got meaningful text and no images -> text_only (fast path)
	if pc.TextCharCount > threshold && !pc.HasImages {
		return ClassTextOnly, 0.95
	}

	// No fonts and has large images -> scanned document
	if !pc.HasEmbeddedFonts && pc.LargeImageCount > 0 {
		return ClassScanned, 0.90
	}

	// Has some text and has images -> hybrid
	if pc.TextCharCount > threshold/2 && pc.HasImages {
		return ClassHybrid, 0.85
	}

	// Has images but little/no text -> image_only (needs OCR)
	if pc.HasImages && pc.TextCharCount < threshold/2 {
		return ClassImageOnly, 0.85
	}

	// Almost no text and no images -> empty page
	if pc.TextCharCount < 10 && !pc.HasImages {
		return ClassEmpty, 0.95
	}

	// Has some text (even with fonts) -> text_only as fallback
	if pc.TextCharCount >= 10 {
		return ClassTextOnly, 0.70
	}

	// Default: unknown, will try OCR
	return ClassUnknown, 0.50
}

// GetPageCount returns the total page count using pdfinfo
func GetPageCount(ctx context.Context, pdfPath string) (int, error) {
	cmd := exec.CommandContext(ctx, "pdfinfo", pdfPath)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("pdfinfo failed: %w", err)
	}

	// Parse pdfinfo output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "Pages" {
			count, err := strconv.Atoi(value)
			if err != nil {
				return 0, fmt.Errorf("failed to parse page count: %w", err)
			}
			return count, nil
		}
	}

	return 0, fmt.Errorf("page count not found in pdfinfo output")
}
