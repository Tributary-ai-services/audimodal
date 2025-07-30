package markdown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/preprocessing"
)

// MarkdownReader implements DataSourceReader for Markdown files with metadata extraction
type MarkdownReader struct {
	name             string
	version          string
	encodingDetector *preprocessing.EncodingDetector
}

// NewMarkdownReader creates a new Markdown file reader
func NewMarkdownReader() *MarkdownReader {
	return &MarkdownReader{
		name:             "markdown_reader",
		version:          "1.0.0",
		encodingDetector: preprocessing.NewEncodingDetector(),
	}
}

// GetConfigSpec returns the configuration specification
func (r *MarkdownReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_frontmatter",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract YAML/TOML frontmatter metadata",
		},
		{
			Name:        "chunk_by",
			Type:        "string",
			Required:    false,
			Default:     "section",
			Description: "How to chunk the markdown content",
			Enum:        []string{"section", "paragraph", "heading", "block"},
		},
		{
			Name:        "preserve_formatting",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Preserve markdown formatting in chunks",
		},
		{
			Name:        "extract_code_blocks",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract code blocks as separate chunks",
		},
		{
			Name:        "extract_tables",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract markdown tables as structured data",
		},
		{
			Name:        "extract_links",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract and catalog links",
		},
		{
			Name:        "extract_images",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract image references",
		},
		{
			Name:        "min_heading_level",
			Type:        "int",
			Required:    false,
			Default:     1,
			Description: "Minimum heading level to use for chunking",
			MinValue:    ptrFloat64(1.0),
			MaxValue:    ptrFloat64(6.0),
		},
		{
			Name:        "max_heading_level",
			Type:        "int",
			Required:    false,
			Default:     6,
			Description: "Maximum heading level to use for chunking",
			MinValue:    ptrFloat64(1.0),
			MaxValue:    ptrFloat64(6.0),
		},
		{
			Name:        "include_toc",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Generate table of contents from headings",
		},
		{
			Name:        "encoding",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "Markdown file encoding (auto-detect if 'auto')",
			Enum:        []string{"auto", "utf-8", "utf-16", "iso-8859-1", "windows-1252"},
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *MarkdownReader) ValidateConfig(config map[string]any) error {
	if minLevel, ok := config["min_heading_level"]; ok {
		if num, ok := minLevel.(float64); ok {
			if num < 1 || num > 6 {
				return fmt.Errorf("min_heading_level must be between 1 and 6")
			}
		}
	}

	if maxLevel, ok := config["max_heading_level"]; ok {
		if num, ok := maxLevel.(float64); ok {
			if num < 1 || num > 6 {
				return fmt.Errorf("max_heading_level must be between 1 and 6")
			}
		}
	}

	if chunkBy, ok := config["chunk_by"]; ok {
		if str, ok := chunkBy.(string); ok {
			validModes := []string{"section", "paragraph", "heading", "block"}
			found := false
			for _, valid := range validModes {
				if str == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid chunk_by mode: %s", str)
			}
		}
	}

	if encoding, ok := config["encoding"]; ok {
		if str, ok := encoding.(string); ok {
			validEncodings := []string{"auto", "utf-8", "utf-16", "iso-8859-1", "windows-1252"}
			found := false
			for _, valid := range validEncodings {
				if str == valid {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("invalid encoding: %s", str)
			}
		}
	}

	return nil
}

// TestConnection tests if the Markdown can be read
func (r *MarkdownReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
	start := time.Now()

	err := r.ValidateConfig(config)
	if err != nil {
		return core.ConnectionTestResult{
			Success: false,
			Message: "Configuration validation failed",
			Latency: time.Since(start),
			Errors:  []string{err.Error()},
		}
	}

	return core.ConnectionTestResult{
		Success: true,
		Message: "Markdown reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_frontmatter": config["extract_frontmatter"],
			"chunk_by":            config["chunk_by"],
			"extract_code_blocks": config["extract_code_blocks"],
		},
	}
}

// GetType returns the connector type
func (r *MarkdownReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *MarkdownReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *MarkdownReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the Markdown file structure
func (r *MarkdownReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	// Detect encoding first
	encodingInfo, err := r.encodingDetector.DetectEncoding(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to detect encoding: %w", err)
	}

	// Convert to UTF-8 if needed
	var filePath string
	if encodingInfo.Name != "utf-8" {
		filePath, err = r.encodingDetector.ConvertToUTF8(sourcePath, encodingInfo.Name)
		if err != nil {
			return core.SchemaInfo{}, fmt.Errorf("failed to convert encoding: %w", err)
		}
		defer os.Remove(filePath) // Clean up temp file
	} else {
		filePath = sourcePath
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to read file: %w", err)
	}

	mdInfo := r.analyzeMarkdownStructure(string(content))

	schema := core.SchemaInfo{
		Format:   "markdown",
		Encoding: encodingInfo.Name,
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Markdown content chunk",
			},
			{
				Name:        "chunk_type",
				Type:        "string",
				Nullable:    false,
				Description: "Type of markdown element (heading, paragraph, code, table, etc.)",
			},
			{
				Name:        "heading_level",
				Type:        "integer",
				Nullable:    true,
				Description: "Heading level (1-6) if chunk is a heading",
			},
			{
				Name:        "section_title",
				Type:        "string",
				Nullable:    true,
				Description: "Title of the section this chunk belongs to",
			},
			{
				Name:        "language",
				Type:        "string",
				Nullable:    true,
				Description: "Programming language for code blocks",
			},
		},
		Metadata: map[string]any{
			"detected_encoding":  encodingInfo.Name,
			"encoding_confidence": encodingInfo.Confidence,
			"frontmatter":        mdInfo.Frontmatter,
			"heading_count":      mdInfo.HeadingCount,
			"code_block_count":   mdInfo.CodeBlockCount,
			"table_count":        mdInfo.TableCount,
			"link_count":         mdInfo.LinkCount,
			"image_count":        mdInfo.ImageCount,
			"word_count":         mdInfo.WordCount,
			"max_heading_level":  mdInfo.MaxHeadingLevel,
			"has_toc":            mdInfo.HasTOC,
			"languages_found":    mdInfo.LanguagesFound,
		},
	}

	// Extract sample content
	if len(mdInfo.SampleChunks) > 0 {
		sampleData := make([]map[string]any, 0, min(len(mdInfo.SampleChunks), 3))
		for i, chunk := range mdInfo.SampleChunks {
			if i >= 3 {
				break
			}
			sampleData = append(sampleData, map[string]any{
				"content":       chunk.Content,
				"chunk_type":    chunk.Type,
				"heading_level": chunk.HeadingLevel,
				"section_title": chunk.SectionTitle,
				"language":      chunk.Language,
			})
		}
		schema.SampleData = sampleData
	}

	return schema, nil
}

// EstimateSize returns size estimates for the Markdown file
func (r *MarkdownReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to read file: %w", err)
	}

	mdInfo := r.analyzeMarkdownStructure(string(content))

	// Estimate chunks based on structure
	estimatedChunks := mdInfo.HeadingCount + mdInfo.CodeBlockCount + mdInfo.TableCount
	if estimatedChunks == 0 {
		// Estimate based on paragraphs
		paragraphs := strings.Count(string(content), "\n\n")
		estimatedChunks = max(paragraphs/2, 1)
	}

	complexity := "low"
	if stat.Size() > 100*1024 || mdInfo.HeadingCount > 50 { // > 100KB or > 50 headings
		complexity = "medium"
	}
	if stat.Size() > 1*1024*1024 || mdInfo.HeadingCount > 200 { // > 1MB or > 200 headings
		complexity = "high"
	}

	processTime := "fast"
	if stat.Size() > 500*1024 || mdInfo.CodeBlockCount > 50 {
		processTime = "medium"
	}
	if stat.Size() > 5*1024*1024 || mdInfo.CodeBlockCount > 200 {
		processTime = "slow"
	}

	estimatedRows := int64(estimatedChunks)
	return core.SizeEstimate{
		RowCount:    &estimatedRows,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the Markdown file
func (r *MarkdownReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	// Detect and handle encoding
	var filePath string
	var cleanupPath string
	
	if encoding, ok := strategyConfig["encoding"].(string); ok && encoding != "auto" {
		// Use specified encoding
		if encoding != "utf-8" {
			convertedPath, err := r.encodingDetector.ConvertToUTF8(sourcePath, encoding)
			if err != nil {
				return nil, fmt.Errorf("failed to convert from %s encoding: %w", encoding, err)
			}
			filePath = convertedPath
			cleanupPath = convertedPath
		} else {
			filePath = sourcePath
		}
	} else {
		// Auto-detect encoding
		encodingInfo, err := r.encodingDetector.DetectEncoding(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("failed to detect encoding: %w", err)
		}
		
		if encodingInfo.Name != "utf-8" {
			convertedPath, err := r.encodingDetector.ConvertToUTF8(sourcePath, encodingInfo.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to convert from %s encoding: %w", encodingInfo.Name, err)
			}
			filePath = convertedPath
			cleanupPath = convertedPath
		} else {
			filePath = sourcePath
		}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		if cleanupPath != "" {
			os.Remove(cleanupPath)
		}
		return nil, fmt.Errorf("failed to read markdown file: %w", err)
	}

	chunks := r.parseMarkdownChunks(string(content), strategyConfig)
	
	iterator := &MarkdownIterator{
		sourcePath:   sourcePath,
		cleanupPath:  cleanupPath,
		config:       strategyConfig,
		chunks:       chunks,
		currentChunk: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates Markdown reader supports streaming
func (r *MarkdownReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *MarkdownReader) GetSupportedFormats() []string {
	return []string{"md", "markdown", "mdown", "mkd", "mdx"}
}

// MarkdownInfo contains analyzed markdown structure information
type MarkdownInfo struct {
	Frontmatter      map[string]any
	HeadingCount     int
	CodeBlockCount   int
	TableCount       int
	LinkCount        int
	ImageCount       int
	WordCount        int
	MaxHeadingLevel  int
	HasTOC           bool
	LanguagesFound   []string
	SampleChunks     []MarkdownChunk
}

// MarkdownChunk represents a parsed markdown chunk
type MarkdownChunk struct {
	Content      string
	Type         string
	HeadingLevel int
	SectionTitle string
	Language     string
	LineNumber   int
}

// analyzeMarkdownStructure analyzes the structure of a markdown file
func (r *MarkdownReader) analyzeMarkdownStructure(content string) MarkdownInfo {
	info := MarkdownInfo{
		LanguagesFound: make([]string, 0),
		SampleChunks:   make([]MarkdownChunk, 0),
	}

	lines := strings.Split(content, "\n")
	var frontmatterEnd int

	// Extract frontmatter
	if len(lines) > 0 && strings.HasPrefix(lines[0], "---") {
		for i := 1; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "---") {
				frontmatterEnd = i + 1
				frontmatterContent := strings.Join(lines[1:i], "\n")
				info.Frontmatter = r.parseFrontmatter(frontmatterContent)
				break
			}
		}
	}

	// Analyze content after frontmatter
	contentLines := lines[frontmatterEnd:]
	sampleCount := 0

	for i, line := range contentLines {
		// Count headings
		if strings.HasPrefix(line, "#") {
			level := 0
			for _, char := range line {
				if char == '#' {
					level++
				} else {
					break
				}
			}
			if level <= 6 {
				info.HeadingCount++
				if level > info.MaxHeadingLevel {
					info.MaxHeadingLevel = level
				}
				
				// Add as sample chunk
				if sampleCount < 5 {
					info.SampleChunks = append(info.SampleChunks, MarkdownChunk{
						Content:      strings.TrimSpace(line[level:]),
						Type:         "heading",
						HeadingLevel: level,
						LineNumber:   i + frontmatterEnd + 1,
					})
					sampleCount++
				}
			}
		}

		// Count code blocks
		if strings.HasPrefix(line, "```") {
			info.CodeBlockCount++
			// Extract language
			lang := strings.TrimSpace(line[3:])
			if lang != "" {
				found := false
				for _, existing := range info.LanguagesFound {
					if existing == lang {
						found = true
						break
					}
				}
				if !found {
					info.LanguagesFound = append(info.LanguagesFound, lang)
				}
			}
		}

		// Count tables (lines starting with |)
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			info.TableCount++
		}

		// Count links [text](url)
		linkPattern := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
		matches := linkPattern.FindAllString(line, -1)
		info.LinkCount += len(matches)

		// Count images ![alt](url)
		imagePattern := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
		imageMatches := imagePattern.FindAllString(line, -1)
		info.ImageCount += len(imageMatches)
	}

	// Count words (rough estimate)
	words := strings.Fields(strings.Join(contentLines, " "))
	info.WordCount = len(words)

	// Check for TOC indicators
	toc := strings.ToLower(content)
	info.HasTOC = strings.Contains(toc, "table of contents") || 
		         strings.Contains(toc, "toc") ||
		         strings.Contains(toc, "- [") // Common TOC pattern

	return info
}

// parseFrontmatter parses YAML frontmatter (simplified)
func (r *MarkdownReader) parseFrontmatter(content string) map[string]any {
	frontmatter := make(map[string]any)
	
	// Simple key-value parsing for YAML-like frontmatter
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				
				// Remove quotes if present
				if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				   (strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
					value = value[1 : len(value)-1]
				}
				
				frontmatter[key] = value
			}
		}
	}
	
	return frontmatter
}

// parseMarkdownChunks parses markdown into chunks based on configuration
func (r *MarkdownReader) parseMarkdownChunks(content string, config map[string]any) []MarkdownChunk {
	var chunks []MarkdownChunk
	
	chunkBy := "section"
	if cb, ok := config["chunk_by"].(string); ok {
		chunkBy = cb
	}
	
	lines := strings.Split(content, "\n")
	var frontmatterEnd int
	
	// Skip frontmatter
	if len(lines) > 0 && strings.HasPrefix(lines[0], "---") {
		for i := 1; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "---") {
				frontmatterEnd = i + 1
				
				// Add frontmatter as chunk if requested
				if extractFM, ok := config["extract_frontmatter"].(bool); !ok || extractFM {
					fmContent := strings.Join(lines[1:i], "\n")
					if strings.TrimSpace(fmContent) != "" {
						chunks = append(chunks, MarkdownChunk{
							Content:    fmContent,
							Type:       "frontmatter",
							LineNumber: 1,
						})
					}
				}
				break
			}
		}
	}
	
	contentLines := lines[frontmatterEnd:]
	
	switch chunkBy {
	case "heading":
		chunks = append(chunks, r.chunkByHeading(contentLines, config, frontmatterEnd)...)
	case "paragraph":
		chunks = append(chunks, r.chunkByParagraph(contentLines, config, frontmatterEnd)...)
	case "block":
		chunks = append(chunks, r.chunkByBlock(contentLines, config, frontmatterEnd)...)
	default: // section
		chunks = append(chunks, r.chunkBySection(contentLines, config, frontmatterEnd)...)
	}
	
	return chunks
}

// chunkBySection groups content by sections (between headings)
func (r *MarkdownReader) chunkBySection(lines []string, config map[string]any, offset int) []MarkdownChunk {
	var chunks []MarkdownChunk
	var currentSection []string
	var currentHeading string
	var currentHeadingLevel int
	var sectionStart int
	
	for i, line := range lines {
		if strings.HasPrefix(line, "#") {
			// Save previous section
			if len(currentSection) > 0 {
				content := strings.TrimSpace(strings.Join(currentSection, "\n"))
				if content != "" {
					chunks = append(chunks, MarkdownChunk{
						Content:      content,
						Type:         "section",
						SectionTitle: currentHeading,
						HeadingLevel: currentHeadingLevel,
						LineNumber:   sectionStart + offset + 1,
					})
				}
			}
			
			// Start new section
			level := r.getHeadingLevel(line)
			currentHeading = strings.TrimSpace(line[level:])
			currentHeadingLevel = level
			currentSection = []string{}
			sectionStart = i
			
			// Add heading as separate chunk if configured
			if preserveFormatting, ok := config["preserve_formatting"].(bool); ok && preserveFormatting {
				chunks = append(chunks, MarkdownChunk{
					Content:      line,
					Type:         "heading",
					HeadingLevel: level,
					SectionTitle: currentHeading,
					LineNumber:   i + offset + 1,
				})
			} else {
				chunks = append(chunks, MarkdownChunk{
					Content:      currentHeading,
					Type:         "heading",
					HeadingLevel: level,
					SectionTitle: currentHeading,
					LineNumber:   i + offset + 1,
				})
			}
		} else {
			currentSection = append(currentSection, line)
		}
	}
	
	// Add final section
	if len(currentSection) > 0 {
		content := strings.TrimSpace(strings.Join(currentSection, "\n"))
		if content != "" {
			chunks = append(chunks, MarkdownChunk{
				Content:      content,
				Type:         "section",
				SectionTitle: currentHeading,
				HeadingLevel: currentHeadingLevel,
				LineNumber:   sectionStart + offset + 1,
			})
		}
	}
	
	return chunks
}

// chunkByHeading creates one chunk per heading
func (r *MarkdownReader) chunkByHeading(lines []string, config map[string]any, offset int) []MarkdownChunk {
	var chunks []MarkdownChunk
	
	for i, line := range lines {
		if strings.HasPrefix(line, "#") {
			level := r.getHeadingLevel(line)
			heading := strings.TrimSpace(line[level:])
			
			chunks = append(chunks, MarkdownChunk{
				Content:      heading,
				Type:         "heading",
				HeadingLevel: level,
				SectionTitle: heading,
				LineNumber:   i + offset + 1,
			})
		}
	}
	
	return chunks
}

// chunkByParagraph creates chunks for paragraphs
func (r *MarkdownReader) chunkByParagraph(lines []string, config map[string]any, offset int) []MarkdownChunk {
	var chunks []MarkdownChunk
	var currentParagraph []string
	var paragraphStart int
	
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			// End of paragraph
			if len(currentParagraph) > 0 {
				content := strings.TrimSpace(strings.Join(currentParagraph, "\n"))
				if content != "" {
					chunkType := "paragraph"
					if strings.HasPrefix(currentParagraph[0], "#") {
						chunkType = "heading"
					}
					
					chunks = append(chunks, MarkdownChunk{
						Content:    content,
						Type:       chunkType,
						LineNumber: paragraphStart + offset + 1,
					})
				}
				currentParagraph = []string{}
			}
		} else {
			if len(currentParagraph) == 0 {
				paragraphStart = i
			}
			currentParagraph = append(currentParagraph, line)
		}
	}
	
	// Add final paragraph
	if len(currentParagraph) > 0 {
		content := strings.TrimSpace(strings.Join(currentParagraph, "\n"))
		if content != "" {
			chunkType := "paragraph"
			if strings.HasPrefix(currentParagraph[0], "#") {
				chunkType = "heading"
			}
			
			chunks = append(chunks, MarkdownChunk{
				Content:    content,
				Type:       chunkType,
				LineNumber: paragraphStart + offset + 1,
			})
		}
	}
	
	return chunks
}

// chunkByBlock creates chunks for different markdown blocks
func (r *MarkdownReader) chunkByBlock(lines []string, config map[string]any, offset int) []MarkdownChunk {
	var chunks []MarkdownChunk
	inCodeBlock := false
	var currentBlock []string
	var blockStart int
	var language string
	
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End of code block
				content := strings.Join(currentBlock, "\n")
				chunks = append(chunks, MarkdownChunk{
					Content:    content,
					Type:       "code",
					Language:   language,
					LineNumber: blockStart + offset + 1,
				})
				inCodeBlock = false
				currentBlock = []string{}
			} else {
				// Start of code block
				inCodeBlock = true
				language = strings.TrimSpace(line[3:])
				blockStart = i
				currentBlock = []string{}
			}
		} else if inCodeBlock {
			currentBlock = append(currentBlock, line)
		} else {
			// Regular content
			if strings.HasPrefix(line, "#") {
				// Heading
				level := r.getHeadingLevel(line)
				heading := strings.TrimSpace(line[level:])
				chunks = append(chunks, MarkdownChunk{
					Content:      heading,
					Type:         "heading",
					HeadingLevel: level,
					LineNumber:   i + offset + 1,
				})
			} else if strings.TrimSpace(line) != "" {
				// Regular paragraph line
				chunks = append(chunks, MarkdownChunk{
					Content:    line,
					Type:       "text",
					LineNumber: i + offset + 1,
				})
			}
		}
	}
	
	// Handle unclosed code block
	if inCodeBlock && len(currentBlock) > 0 {
		content := strings.Join(currentBlock, "\n")
		chunks = append(chunks, MarkdownChunk{
			Content:    content,
			Type:       "code",
			Language:   language,
			LineNumber: blockStart + offset + 1,
		})
	}
	
	return chunks
}

// getHeadingLevel returns the heading level (number of # characters)
func (r *MarkdownReader) getHeadingLevel(line string) int {
	level := 0
	for _, char := range line {
		if char == '#' {
			level++
		} else {
			break
		}
	}
	if level > 6 {
		return 6
	}
	return level
}

// MarkdownIterator implements ChunkIterator for Markdown files
type MarkdownIterator struct {
	sourcePath   string
	cleanupPath  string
	config       map[string]any
	chunks       []MarkdownChunk
	currentChunk int
}

// Next returns the next chunk of content from the Markdown
func (it *MarkdownIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Check if we've processed all chunks
	if it.currentChunk >= len(it.chunks) {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	chunk := it.chunks[it.currentChunk]
	it.currentChunk++

	coreChunk := core.Chunk{
		Data: chunk.Content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:%s:%d", filepath.Base(it.sourcePath), chunk.Type, it.currentChunk),
			ChunkType:   "markdown_" + chunk.Type,
			SizeBytes:   int64(len(chunk.Content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "markdown_reader",
			Context: map[string]string{
				"chunk_number":   strconv.Itoa(it.currentChunk),
				"total_chunks":   strconv.Itoa(len(it.chunks)),
				"chunk_type":     chunk.Type,
				"line_number":    strconv.Itoa(chunk.LineNumber),
			},
		},
	}

	// Add type-specific context
	if chunk.HeadingLevel > 0 {
		coreChunk.Metadata.Context["heading_level"] = strconv.Itoa(chunk.HeadingLevel)
	}
	if chunk.SectionTitle != "" {
		coreChunk.Metadata.Context["section_title"] = chunk.SectionTitle
	}
	if chunk.Language != "" {
		coreChunk.Metadata.Context["language"] = chunk.Language
	}

	return coreChunk, nil
}

// Close releases Markdown resources
func (it *MarkdownIterator) Close() error {
	// Clean up temporary encoding conversion file
	if it.cleanupPath != "" {
		os.Remove(it.cleanupPath)
	}
	return nil
}

// Reset restarts iteration from the beginning
func (it *MarkdownIterator) Reset() error {
	it.currentChunk = 0
	return nil
}

// Progress returns iteration progress
func (it *MarkdownIterator) Progress() float64 {
	if len(it.chunks) == 0 {
		return 1.0
	}
	return float64(it.currentChunk) / float64(len(it.chunks))
}

// Helper functions
func ptrFloat64(f float64) *float64 {
	return &f
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}