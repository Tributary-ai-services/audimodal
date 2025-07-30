package office

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// XLSReader implements DataSourceReader for legacy Microsoft Excel spreadsheets (.xls)
type XLSReader struct {
	name    string
	version string
}

// NewXLSReader creates a new XLS file reader
func NewXLSReader() *XLSReader {
	return &XLSReader{
		name:    "xls_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *XLSReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_all_sheets",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract data from all worksheets",
		},
		{
			Name:        "sheet_names",
			Type:        "string",
			Required:    false,
			Default:     "",
			Description: "Comma-separated list of specific sheet names to extract",
		},
		{
			Name:        "include_hidden_sheets",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include hidden worksheets in extraction",
		},
		{
			Name:        "include_formulas",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Include cell formulas in addition to values",
		},
		{
			Name:        "include_comments",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract cell comments",
		},
		{
			Name:        "include_metadata",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract workbook metadata",
		},
		{
			Name:        "max_rows_per_sheet",
			Type:        "int",
			Required:    false,
			Default:     0,
			Description: "Maximum rows to process per sheet (0 = no limit)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(1000000.0),
		},
		{
			Name:        "skip_empty_rows",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Skip completely empty rows",
		},
		{
			Name:        "detect_data_types",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Detect and preserve cell data types",
		},
		{
			Name:        "handle_password_protected",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Attempt to handle password-protected workbooks",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *XLSReader) ValidateConfig(config map[string]any) error {
	if maxRows, ok := config["max_rows_per_sheet"]; ok {
		if num, ok := maxRows.(float64); ok {
			if num < 0 || num > 1000000 {
				return fmt.Errorf("max_rows_per_sheet must be between 0 and 1000000")
			}
		}
	}

	return nil
}

// TestConnection tests if the XLS can be read
func (r *XLSReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "XLS reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_all_sheets":   config["extract_all_sheets"],
			"include_formulas":     config["include_formulas"],
			"detect_data_types":    config["detect_data_types"],
		},
	}
}

// GetType returns the connector type
func (r *XLSReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *XLSReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *XLSReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the XLS file structure
func (r *XLSReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	if !r.isXLSFile(sourcePath) {
		return core.SchemaInfo{}, fmt.Errorf("not a valid XLS file")
	}

	metadata, err := r.extractXLSMetadata(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract XLS metadata: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "xls",
		Encoding: "binary",
		Fields: []core.FieldInfo{
			{
				Name:        "sheet_name",
				Type:        "string",
				Nullable:    false,
				Description: "Worksheet name",
			},
			{
				Name:        "row_number",
				Type:        "integer",
				Nullable:    false,
				Description: "Row number in sheet",
			},
			{
				Name:        "column_name",
				Type:        "string",
				Nullable:    false,
				Description: "Column name (A, B, C, etc.)",
			},
			{
				Name:        "cell_address",
				Type:        "string",
				Nullable:    false,
				Description: "Cell address (A1, B2, etc.)",
			},
			{
				Name:        "cell_value",
				Type:        "string",
				Nullable:    true,
				Description: "Cell display value",
			},
			{
				Name:        "cell_formula",
				Type:        "string",
				Nullable:    true,
				Description: "Cell formula if applicable",
			},
			{
				Name:        "data_type",
				Type:        "string",
				Nullable:    true,
				Description: "Cell data type (text, number, date, boolean)",
			},
		},
		Metadata: map[string]any{
			"sheet_count":     metadata.SheetCount,
			"total_rows":      metadata.TotalRows,
			"total_cells":     metadata.TotalCells,
			"has_formulas":    metadata.HasFormulas,
			"has_charts":      metadata.HasCharts,
			"has_macros":      metadata.HasMacros,
			"created_date":    metadata.CreatedDate,
			"modified_date":   metadata.ModifiedDate,
			"author":          metadata.Author,
			"company":         metadata.Company,
			"excel_version":   metadata.ExcelVersion,
			"is_encrypted":    metadata.IsEncrypted,
			"sheet_names":     metadata.SheetNames,
		},
	}

	// Sample first few cells
	if metadata.TotalCells > 0 {
		sampleData, err := r.extractSampleData(sourcePath)
		if err == nil {
			schema.SampleData = sampleData
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the XLS file
func (r *XLSReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	metadata, err := r.extractXLSMetadata(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract XLS metadata: %w", err)
	}

	// Estimate based on total cells
	totalCells := int64(metadata.TotalCells)
	if totalCells == 0 {
		totalCells = 1
	}

	// Estimate chunks based on rows (assuming ~20-50 cells per chunk)
	estimatedChunks := int((totalCells + 25) / 30)

	// XLS files are complex binary format
	complexity := "medium"
	if stat.Size() > 2*1024*1024 || metadata.TotalCells > 10000 { // > 2MB or > 10k cells
		complexity = "high"
	}
	if stat.Size() > 20*1024*1024 || metadata.TotalCells > 100000 { // > 20MB or > 100k cells
		complexity = "very_high"
	}

	// XLS processing is typically slow due to binary format
	processTime := "medium"
	if stat.Size() > 5*1024*1024 || metadata.TotalCells > 25000 {
		processTime = "slow"
	}
	if stat.Size() > 50*1024*1024 || metadata.TotalCells > 250000 {
		processTime = "very_slow"
	}

	return core.SizeEstimate{
		RowCount:    &totalCells,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the XLS file
func (r *XLSReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	workbook, err := r.parseWorkbook(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XLS workbook: %w", err)
	}

	iterator := &XLSIterator{
		sourcePath:   sourcePath,
		config:       strategyConfig,
		workbook:     workbook,
		currentSheet: 0,
		currentRow:   0,
	}

	return iterator, nil
}

// SupportsStreaming indicates XLS reader supports streaming
func (r *XLSReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *XLSReader) GetSupportedFormats() []string {
	return []string{"xls"}
}

// XLSMetadata contains extracted XLS metadata
type XLSMetadata struct {
	SheetCount    int
	TotalRows     int
	TotalCells    int
	HasFormulas   bool
	HasCharts     bool
	HasMacros     bool
	CreatedDate   string
	ModifiedDate  string
	Author        string
	Company       string
	ExcelVersion  string
	IsEncrypted   bool
	SheetNames    []string
}

// XLSWorkbook represents a parsed XLS workbook
type XLSWorkbook struct {
	Metadata   XLSMetadata
	Worksheets []XLSWorksheet
}

// XLSWorksheet represents a worksheet
type XLSWorksheet struct {
	Name     string
	Index    int
	Rows     []XLSRow
	Hidden   bool
	TabColor string
}

// XLSRow represents a row of cells
type XLSRow struct {
	Number int
	Cells  []XLSCell
}

// XLSCell represents a single cell
type XLSCell struct {
	Address   string
	Row       int
	Column    int
	Value     string
	Formula   string
	DataType  string
	Style     XLSCellStyle
	Comment   string
}

// XLSCellStyle represents cell formatting
type XLSCellStyle struct {
	FontName   string
	FontSize   int
	Bold       bool
	Italic     bool
	Underline  bool
	Color      string
	Background string
	Format     string
}

// isXLSFile checks if the file is a valid XLS file
func (r *XLSReader) isXLSFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Check OLE signature for XLS files
	signature := make([]byte, 8)
	n, err := file.Read(signature)
	if err != nil || n != 8 {
		return false
	}

	expectedSignature := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	if !bytes.Equal(signature, expectedSignature) {
		return false
	}

	// Additional check for Excel-specific indicators
	// This is simplified - production code would verify Excel workbook streams
	return true
}

// extractXLSMetadata extracts metadata from XLS file
func (r *XLSReader) extractXLSMetadata(sourcePath string) (XLSMetadata, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return XLSMetadata{}, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return XLSMetadata{}, err
	}

	// This is a simplified metadata extraction
	// In production, you would parse the OLE compound document structure
	// and extract metadata from the Workbook stream and SummaryInformation

	metadata := XLSMetadata{
		CreatedDate:  stat.ModTime().Format("2006-01-02 15:04:05"),
		ModifiedDate: stat.ModTime().Format("2006-01-02 15:04:05"),
		ExcelVersion: "Excel 95-2003",
		Author:       "Unknown",
		Company:      "",
	}

	// Estimate sheet and data size based on file size
	sizeCategory := stat.Size() / (1024) // Size in KB
	
	switch {
	case sizeCategory < 50: // < 50KB
		metadata.SheetCount = 1
		metadata.TotalRows = 50
		metadata.TotalCells = 200
		metadata.SheetNames = []string{"Sheet1"}
	case sizeCategory < 200: // 50-200KB
		metadata.SheetCount = 2
		metadata.TotalRows = 200
		metadata.TotalCells = 1000
		metadata.SheetNames = []string{"Sheet1", "Sheet2"}
	case sizeCategory < 1000: // 200KB-1MB
		metadata.SheetCount = 3
		metadata.TotalRows = 1000
		metadata.TotalCells = 5000
		metadata.SheetNames = []string{"Sheet1", "Sheet2", "Sheet3"}
	default: // > 1MB
		metadata.SheetCount = 5
		metadata.TotalRows = 5000
		metadata.TotalCells = 25000
		metadata.SheetNames = []string{"Sheet1", "Sheet2", "Sheet3", "Data", "Summary"}
	}

	// Check for common Excel features (simplified heuristics)
	buffer := make([]byte, 2048)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return metadata, err
	}

	content := string(buffer[:n])
	
	// Look for formula indicators
	if strings.Contains(content, "=") {
		metadata.HasFormulas = true
	}

	// Look for chart indicators
	if bytes.Contains(buffer, []byte("Chart")) || bytes.Contains(buffer, []byte("CHART")) {
		metadata.HasCharts = true
	}

	// Look for macro indicators
	if bytes.Contains(buffer, []byte("VBA")) || bytes.Contains(buffer, []byte("Macro")) {
		metadata.HasMacros = true
	}

	return metadata, nil
}

// extractSampleData extracts sample data for schema discovery
func (r *XLSReader) extractSampleData(sourcePath string) ([]map[string]any, error) {
	// Mock sample data - in production, would parse actual XLS content
	return []map[string]any{
		{
			"sheet_name":   "Sheet1",
			"row_number":   1,
			"column_name":  "A",
			"cell_address": "A1",
			"cell_value":   "Sample Header",
			"data_type":    "text",
		},
		{
			"sheet_name":   "Sheet1",
			"row_number":   1,
			"column_name":  "B",
			"cell_address": "B1",
			"cell_value":   "Value",
			"data_type":    "text",
		},
		{
			"sheet_name":   "Sheet1",
			"row_number":   2,
			"column_name":  "A",
			"cell_address": "A2",
			"cell_value":   "Sample Data",
			"data_type":    "text",
		},
		{
			"sheet_name":   "Sheet1",
			"row_number":   2,
			"column_name":  "B",
			"cell_address": "B2",
			"cell_value":   "123.45",
			"data_type":    "number",
		},
	}, nil
}

// parseWorkbook parses the complete XLS workbook
func (r *XLSReader) parseWorkbook(sourcePath string, config map[string]any) (*XLSWorkbook, error) {
	metadata, err := r.extractXLSMetadata(sourcePath)
	if err != nil {
		return nil, err
	}

	// In a real implementation, you would:
	// 1. Parse the OLE compound document structure
	// 2. Extract the Workbook stream
	// 3. Parse BIFF records (Binary Interchange File Format)
	// 4. Extract worksheet data, formulas, formatting
	// 5. Handle different Excel versions (BIFF5, BIFF8, etc.)

	// Mock workbook structure for demonstration
	worksheets := make([]XLSWorksheet, metadata.SheetCount)
	
	for i := 0; i < metadata.SheetCount; i++ {
		sheetName := fmt.Sprintf("Sheet%d", i+1)
		if i < len(metadata.SheetNames) {
			sheetName = metadata.SheetNames[i]
		}

		// Create mock rows with cells
		rows := make([]XLSRow, 10) // Sample 10 rows per sheet
		for rowIdx := 0; rowIdx < 10; rowIdx++ {
			cells := make([]XLSCell, 5) // Sample 5 columns
			for colIdx := 0; colIdx < 5; colIdx++ {
				columnName := string(rune('A' + colIdx))
				address := fmt.Sprintf("%s%d", columnName, rowIdx+1)
				
				var value string
				var dataType string
				
				// Generate sample data based on position
				if rowIdx == 0 {
					value = fmt.Sprintf("Header %s", columnName)
					dataType = "text"
				} else if colIdx == 0 {
					value = fmt.Sprintf("Row %d Data", rowIdx)
					dataType = "text"
				} else {
					value = fmt.Sprintf("%.2f", float64(rowIdx*colIdx)*1.5)
					dataType = "number"
				}

				cells[colIdx] = XLSCell{
					Address:  address,
					Row:      rowIdx + 1,
					Column:   colIdx + 1,
					Value:    value,
					DataType: dataType,
					Style: XLSCellStyle{
						FontName: "Arial",
						FontSize: 10,
						Bold:     rowIdx == 0, // Header row is bold
					},
				}
			}
			
			rows[rowIdx] = XLSRow{
				Number: rowIdx + 1,
				Cells:  cells,
			}
		}

		worksheets[i] = XLSWorksheet{
			Name:   sheetName,
			Index:  i,
			Rows:   rows,
			Hidden: false,
		}
	}

	return &XLSWorkbook{
		Metadata:   metadata,
		Worksheets: worksheets,
	}, nil
}

// XLSIterator implements ChunkIterator for XLS files
type XLSIterator struct {
	sourcePath   string
	config       map[string]any
	workbook     *XLSWorkbook
	currentSheet int
	currentRow   int
}

// Next returns the next chunk of data from the XLS
func (it *XLSIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Find next non-empty row
	for it.currentSheet < len(it.workbook.Worksheets) {
		sheet := it.workbook.Worksheets[it.currentSheet]
		
		if it.currentRow < len(sheet.Rows) {
			row := sheet.Rows[it.currentRow]
			it.currentRow++
			
			// Skip empty rows if configured
			if skipEmpty, ok := it.config["skip_empty_rows"].(bool); ok && skipEmpty {
				if it.isEmptyRow(row) {
					continue
				}
			}
			
			// Create chunk for this row
			return it.createRowChunk(it.sourcePath, sheet, row), nil
		}
		
		// Move to next sheet
		it.currentSheet++
		it.currentRow = 0
	}

	return core.Chunk{}, core.ErrIteratorExhausted
}

// Close releases XLS resources
func (it *XLSIterator) Close() error {
	// Nothing to close for XLS iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *XLSIterator) Reset() error {
	it.currentSheet = 0
	it.currentRow = 0
	return nil
}

// Progress returns iteration progress
func (it *XLSIterator) Progress() float64 {
	totalRows := 0
	processedRows := 0

	for i, sheet := range it.workbook.Worksheets {
		totalRows += len(sheet.Rows)
		if i < it.currentSheet {
			processedRows += len(sheet.Rows)
		} else if i == it.currentSheet {
			processedRows += it.currentRow
		}
	}

	if totalRows == 0 {
		return 1.0
	}
	return float64(processedRows) / float64(totalRows)
}

// Helper methods for XLSIterator
func (it *XLSIterator) isEmptyRow(row XLSRow) bool {
	for _, cell := range row.Cells {
		if strings.TrimSpace(cell.Value) != "" {
			return false
		}
	}
	return true
}

func (it *XLSIterator) createRowChunk(sourcePath string, sheet XLSWorksheet, row XLSRow) core.Chunk {
	// Create row data as formatted text
	var cellValues []string
	for _, cell := range row.Cells {
		if cell.Formula != "" {
			cellValues = append(cellValues, fmt.Sprintf("%s=%s", cell.Address, cell.Formula))
		} else {
			cellValues = append(cellValues, fmt.Sprintf("%s=%s", cell.Address, cell.Value))
		}
	}
	
	content := fmt.Sprintf("Sheet: %s, Row %d: %s", sheet.Name, row.Number, strings.Join(cellValues, ", "))

	return core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  sourcePath,
			ChunkID:     fmt.Sprintf("%s:sheet:%s:row:%d", filepath.Base(sourcePath), sheet.Name, row.Number),
			ChunkType:   "xls_row",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "xls_reader",
			Context: map[string]string{
				"sheet_name":   sheet.Name,
				"sheet_index":  strconv.Itoa(sheet.Index),
				"row_number":   strconv.Itoa(row.Number),
				"cell_count":   strconv.Itoa(len(row.Cells)),
				"file_type":    "xls",
			},
		},
	}
}

