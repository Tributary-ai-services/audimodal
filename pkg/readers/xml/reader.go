package xml

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/preprocessing"
)

// XMLReader implements DataSourceReader for XML files with schema detection
type XMLReader struct {
	name             string
	version          string
	encodingDetector *preprocessing.EncodingDetector
}

// NewXMLReader creates a new XML file reader
func NewXMLReader() *XMLReader {
	return &XMLReader{
		name:             "xml_reader",
		version:          "1.0.0",
		encodingDetector: preprocessing.NewEncodingDetector(),
	}
}

// GetConfigSpec returns the configuration specification
func (r *XMLReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_attributes",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract XML element attributes",
		},
		{
			Name:        "preserve_hierarchy",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Preserve XML element hierarchy in output",
		},
		{
			Name:        "max_depth",
			Type:        "int",
			Required:    false,
			Default:     10,
			Description: "Maximum XML nesting depth to process",
			MinValue:    ptrFloat64(1.0),
			MaxValue:    ptrFloat64(50.0),
		},
		{
			Name:        "chunk_by",
			Type:        "string",
			Required:    false,
			Default:     "element",
			Description: "How to chunk the XML content",
			Enum:        []string{"element", "depth", "size"},
		},
		{
			Name:        "target_elements",
			Type:        "array",
			Required:    false,
			Default:     []string{},
			Description: "Specific XML elements to extract (empty = all)",
		},
		{
			Name:        "skip_elements",
			Type:        "array",
			Required:    false,
			Default:     []string{},
			Description: "XML elements to skip during processing",
		},
		{
			Name:        "extract_text_content",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract text content from XML elements",
		},
		{
			Name:        "normalize_whitespace",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Normalize whitespace in text content",
		},
		{
			Name:        "detect_namespaces",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Detect and handle XML namespaces",
		},
		{
			Name:        "encoding",
			Type:        "string",
			Required:    false,
			Default:     "auto",
			Description: "XML file encoding (auto-detect if 'auto')",
			Enum:        []string{"auto", "utf-8", "utf-16", "iso-8859-1", "windows-1252"},
		},
		{
			Name:        "validate_xml",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Validate XML structure during parsing",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *XMLReader) ValidateConfig(config map[string]any) error {
	if maxDepth, ok := config["max_depth"]; ok {
		if num, ok := maxDepth.(float64); ok {
			if num < 1 || num > 50 {
				return fmt.Errorf("max_depth must be between 1 and 50")
			}
		}
	}

	if chunkBy, ok := config["chunk_by"]; ok {
		if str, ok := chunkBy.(string); ok {
			validModes := []string{"element", "depth", "size"}
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

// TestConnection tests if the XML can be read
func (r *XMLReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "XML reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_attributes": config["extract_attributes"],
			"preserve_hierarchy": config["preserve_hierarchy"],
			"detect_namespaces":  config["detect_namespaces"],
		},
	}
}

// GetType returns the connector type
func (r *XMLReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *XMLReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *XMLReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the XML file structure
func (r *XMLReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
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

	file, err := os.Open(filePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to open XML file: %w", err)
	}
	defer file.Close()

	// Analyze XML structure
	xmlInfo, err := r.analyzeXMLStructure(file)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to analyze XML structure: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "xml",
		Encoding: encodingInfo.Name,
		Fields: []core.FieldInfo{
			{
				Name:        "element_name",
				Type:        "string",
				Nullable:    false,
				Description: "XML element name",
			},
			{
				Name:        "content",
				Type:        "text",
				Nullable:    true,
				Description: "XML element text content",
			},
			{
				Name:        "attributes",
				Type:        "object",
				Nullable:    true,
				Description: "XML element attributes",
			},
			{
				Name:        "xpath",
				Type:        "string",
				Nullable:    false,
				Description: "XPath to element",
			},
			{
				Name:        "depth",
				Type:        "integer",
				Nullable:    false,
				Description: "Nesting depth of element",
			},
			{
				Name:        "namespace",
				Type:        "string",
				Nullable:    true,
				Description: "XML namespace",
			},
		},
		Metadata: map[string]any{
			"detected_encoding":   encodingInfo.Name,
			"encoding_confidence": encodingInfo.Confidence,
			"root_element":        xmlInfo.RootElement,
			"total_elements":      xmlInfo.TotalElements,
			"max_depth":           xmlInfo.MaxDepth,
			"unique_elements":     xmlInfo.UniqueElements,
			"has_namespaces":      xmlInfo.HasNamespaces,
			"has_attributes":      xmlInfo.HasAttributes,
			"has_cdata":           xmlInfo.HasCDATA,
			"has_comments":        xmlInfo.HasComments,
			"xml_version":         xmlInfo.XMLVersion,
			"xml_declaration":     xmlInfo.XMLDeclaration,
			"namespaces":          xmlInfo.Namespaces,
			"element_frequency":   xmlInfo.ElementFrequency,
		},
	}

	// Extract sample content
	if xmlInfo.SampleElements != nil && len(xmlInfo.SampleElements) > 0 {
		sampleData := make([]map[string]any, 0, min(len(xmlInfo.SampleElements), 3))
		for i, elem := range xmlInfo.SampleElements {
			if i >= 3 {
				break
			}
			sampleData = append(sampleData, map[string]any{
				"element_name": elem.Name,
				"content":      elem.Content,
				"attributes":   elem.Attributes,
				"xpath":        elem.XPath,
				"depth":        elem.Depth,
				"namespace":    elem.Namespace,
			})
		}
		schema.SampleData = sampleData
	}

	return schema, nil
}

// EstimateSize returns size estimates for the XML file
func (r *XMLReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	// Quick analysis for size estimation
	file, err := os.Open(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to open XML file: %w", err)
	}
	defer file.Close()

	// Read first 64KB for estimation
	buffer := make([]byte, 65536)
	n, _ := file.Read(buffer)
	sample := string(buffer[:n])

	// Estimate element count from sample
	elementCount := strings.Count(sample, "<") - strings.Count(sample, "</") - strings.Count(sample, "<!--")
	if elementCount <= 0 {
		elementCount = 1
	}

	// Extrapolate to full file
	sampleRatio := float64(n) / float64(stat.Size())
	if sampleRatio > 1.0 {
		sampleRatio = 1.0
	}
	estimatedElements := int64(float64(elementCount) / sampleRatio)

	complexity := "low"
	if stat.Size() > 1*1024*1024 || estimatedElements > 1000 { // > 1MB or > 1K elements
		complexity = "medium"
	}
	if stat.Size() > 10*1024*1024 || estimatedElements > 10000 { // > 10MB or > 10K elements
		complexity = "high"
	}

	processTime := "fast"
	if stat.Size() > 5*1024*1024 || estimatedElements > 5000 {
		processTime = "medium"
	}
	if stat.Size() > 50*1024*1024 || estimatedElements > 50000 {
		processTime = "slow"
	}

	estimatedChunks := int(estimatedElements)
	if estimatedChunks == 0 {
		estimatedChunks = 1
	}

	return core.SizeEstimate{
		RowCount:    &estimatedElements,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the XML file
func (r *XMLReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
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

	file, err := os.Open(filePath)
	if err != nil {
		if cleanupPath != "" {
			os.Remove(cleanupPath)
		}
		return nil, fmt.Errorf("failed to open XML file: %w", err)
	}

	iterator := &XMLIterator{
		file:        file,
		sourcePath:  sourcePath,
		actualPath:  filePath,
		cleanupPath: cleanupPath,
		config:      strategyConfig,
		decoder:     xml.NewDecoder(file),
		depth:       0,
		elementPath: []string{},
	}

	return iterator, nil
}

// SupportsStreaming indicates XML reader supports streaming
func (r *XMLReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *XMLReader) GetSupportedFormats() []string {
	return []string{"xml", "xsd", "xsl", "xslt", "rss", "atom", "soap", "wsdl"}
}

// XMLInfo contains analyzed XML structure information
type XMLInfo struct {
	RootElement      string
	TotalElements    int
	MaxDepth         int
	UniqueElements   []string
	HasNamespaces    bool
	HasAttributes    bool
	HasCDATA         bool
	HasComments      bool
	XMLVersion       string
	XMLDeclaration   string
	Namespaces       map[string]string
	ElementFrequency map[string]int
	SampleElements   []XMLElement
}

// XMLElement represents a parsed XML element
type XMLElement struct {
	Name       string
	Content    string
	Attributes map[string]string
	XPath      string
	Depth      int
	Namespace  string
}

// analyzeXMLStructure analyzes the structure of an XML file
func (r *XMLReader) analyzeXMLStructure(reader io.Reader) (XMLInfo, error) {
	info := XMLInfo{
		ElementFrequency: make(map[string]int),
		Namespaces:       make(map[string]string),
		SampleElements:   make([]XMLElement, 0),
	}

	decoder := xml.NewDecoder(reader)
	depth := 0
	elementPath := []string{}
	sampleCount := 0

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return info, fmt.Errorf("failed to parse XML: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			depth++
			if depth > info.MaxDepth {
				info.MaxDepth = depth
			}

			elementName := elem.Name.Local
			if elem.Name.Space != "" {
				info.HasNamespaces = true
				info.Namespaces[elem.Name.Space] = elem.Name.Space
				elementName = elem.Name.Space + ":" + elem.Name.Local
			}

			// Track root element
			if depth == 1 {
				info.RootElement = elementName
			}

			elementPath = append(elementPath, elementName)
			info.ElementFrequency[elementName]++
			info.TotalElements++

			// Check for attributes
			if len(elem.Attr) > 0 {
				info.HasAttributes = true
			}

			// Collect sample elements
			if sampleCount < 10 {
				attributes := make(map[string]string)
				for _, attr := range elem.Attr {
					attributes[attr.Name.Local] = attr.Value
				}

				xpath := "/" + strings.Join(elementPath, "/")

				sampleElement := XMLElement{
					Name:       elementName,
					Attributes: attributes,
					XPath:      xpath,
					Depth:      depth,
					Namespace:  elem.Name.Space,
				}

				info.SampleElements = append(info.SampleElements, sampleElement)
				sampleCount++
			}

		case xml.EndElement:
			if len(elementPath) > 0 {
				elementPath = elementPath[:len(elementPath)-1]
			}
			depth--

		case xml.CharData:
			content := strings.TrimSpace(string(elem))
			if content != "" && sampleCount > 0 && len(info.SampleElements) > 0 {
				// Add content to the last sample element
				lastIdx := len(info.SampleElements) - 1
				if info.SampleElements[lastIdx].Content == "" {
					info.SampleElements[lastIdx].Content = content
				}
			}

		case xml.Comment:
			info.HasComments = true

		case xml.ProcInst:
			if elem.Target == "xml" {
				info.XMLDeclaration = string(elem.Inst)
				// Parse XML version
				if versionStart := strings.Index(string(elem.Inst), "version=\""); versionStart >= 0 {
					versionStart += 9
					versionEnd := strings.Index(string(elem.Inst)[versionStart:], "\"")
					if versionEnd > 0 {
						info.XMLVersion = string(elem.Inst)[versionStart : versionStart+versionEnd]
					}
				}
			}
		}
	}

	// Extract unique elements
	for element := range info.ElementFrequency {
		info.UniqueElements = append(info.UniqueElements, element)
	}

	return info, nil
}

// XMLIterator implements ChunkIterator for XML files
type XMLIterator struct {
	file        *os.File
	sourcePath  string
	actualPath  string
	cleanupPath string
	config      map[string]any
	decoder     *xml.Decoder
	depth       int
	elementPath []string
	chunkCount  int
}

// Next returns the next chunk of content from the XML
func (it *XMLIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	for {
		token, err := it.decoder.Token()
		if err == io.EOF {
			return core.Chunk{}, core.ErrIteratorExhausted
		}
		if err != nil {
			return core.Chunk{}, fmt.Errorf("failed to parse XML: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			it.depth++
			elementName := elem.Name.Local
			namespace := elem.Name.Space

			if namespace != "" {
				elementName = namespace + ":" + elem.Name.Local
			}

			it.elementPath = append(it.elementPath, elementName)

			// Check if we should process this element
			if it.shouldProcessElement(elementName) {
				element, err := it.parseElement(elem)
				if err != nil {
					return core.Chunk{}, fmt.Errorf("failed to parse element: %w", err)
				}

				it.chunkCount++
				return it.createChunk(element), nil
			}

		case xml.EndElement:
			if len(it.elementPath) > 0 {
				it.elementPath = it.elementPath[:len(it.elementPath)-1]
			}
			it.depth--
		}
	}
}

// shouldProcessElement determines if an element should be processed
func (it *XMLIterator) shouldProcessElement(elementName string) bool {
	// Check max depth
	if maxDepth, ok := it.config["max_depth"].(float64); ok {
		if it.depth > int(maxDepth) {
			return false
		}
	}

	// Check skip elements
	if skipElements, ok := it.config["skip_elements"].([]string); ok {
		for _, skip := range skipElements {
			if elementName == skip {
				return false
			}
		}
	}

	// Check target elements (if specified)
	if targetElements, ok := it.config["target_elements"].([]string); ok && len(targetElements) > 0 {
		for _, target := range targetElements {
			if elementName == target {
				return true
			}
		}
		return false
	}

	return true
}

// parseElement parses an XML element into an XMLElement struct
func (it *XMLIterator) parseElement(elem xml.StartElement) (XMLElement, error) {
	elementName := elem.Name.Local
	namespace := elem.Name.Space

	if namespace != "" {
		elementName = namespace + ":" + elem.Name.Local
	}

	// Parse attributes
	attributes := make(map[string]string)
	if extractAttrs, ok := it.config["extract_attributes"].(bool); !ok || extractAttrs {
		for _, attr := range elem.Attr {
			attributes[attr.Name.Local] = attr.Value
		}
	}

	// Parse content
	var content strings.Builder
	for {
		token, err := it.decoder.Token()
		if err != nil {
			return XMLElement{}, err
		}

		switch t := token.(type) {
		case xml.CharData:
			if extractText, ok := it.config["extract_text_content"].(bool); !ok || extractText {
				text := string(t)
				if normalize, ok := it.config["normalize_whitespace"].(bool); !ok || normalize {
					text = strings.TrimSpace(text)
					text = strings.ReplaceAll(text, "\n", " ")
					text = strings.ReplaceAll(text, "\t", " ")
					// Replace multiple spaces with single space
					for strings.Contains(text, "  ") {
						text = strings.ReplaceAll(text, "  ", " ")
					}
				}
				if text != "" {
					content.WriteString(text)
				}
			}

		case xml.EndElement:
			if t.Name == elem.Name {
				// This is the end of our element
				xpath := "/" + strings.Join(it.elementPath, "/")

				return XMLElement{
					Name:       elementName,
					Content:    content.String(),
					Attributes: attributes,
					XPath:      xpath,
					Depth:      it.depth,
					Namespace:  namespace,
				}, nil
			} else {
				// This is a nested element end, continue parsing
				continue
			}

		case xml.StartElement:
			// Skip nested elements for now - we're only parsing the current level
			it.skipElement()
		}
	}
}

// skipElement skips the current element and all its children
func (it *XMLIterator) skipElement() error {
	depth := 1
	for depth > 0 {
		token, err := it.decoder.Token()
		if err != nil {
			return err
		}

		switch token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

// createChunk creates a chunk from an XMLElement
func (it *XMLIterator) createChunk(element XMLElement) core.Chunk {
	chunk := core.Chunk{
		Data: element.Content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:element:%d", filepath.Base(it.sourcePath), it.chunkCount),
			ChunkType:   "xml_element",
			SizeBytes:   int64(len(element.Content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "xml_reader",
			Context: map[string]string{
				"element_name": element.Name,
				"xpath":        element.XPath,
				"depth":        strconv.Itoa(element.Depth),
				"namespace":    element.Namespace,
			},
		},
	}

	// Add attributes to context
	if len(element.Attributes) > 0 {
		for key, value := range element.Attributes {
			chunk.Metadata.Context["attr_"+key] = value
		}
		chunk.Metadata.Context["has_attributes"] = "true"
	}

	return chunk
}

// Close releases XML resources
func (it *XMLIterator) Close() error {
	var err error
	if it.file != nil {
		err = it.file.Close()
	}

	// Clean up temporary encoding conversion file
	if it.cleanupPath != "" {
		os.Remove(it.cleanupPath)
	}

	return err
}

// Reset restarts iteration from the beginning
func (it *XMLIterator) Reset() error {
	if it.file != nil {
		_, err := it.file.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}
		it.decoder = xml.NewDecoder(it.file)
		it.depth = 0
		it.elementPath = []string{}
		it.chunkCount = 0
		return nil
	}
	return fmt.Errorf("file not open")
}

// Progress returns iteration progress (approximation for XML)
func (it *XMLIterator) Progress() float64 {
	if it.file == nil {
		return 0.0
	}

	// Get current position and file size for rough progress
	pos, err := it.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0.0
	}

	stat, err := it.file.Stat()
	if err != nil {
		return 0.0
	}

	if stat.Size() == 0 {
		return 1.0
	}

	return float64(pos) / float64(stat.Size())
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
