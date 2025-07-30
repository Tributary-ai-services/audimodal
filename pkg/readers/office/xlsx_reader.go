package office

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// XLSXReader implements DataSourceReader for Microsoft Excel spreadsheets
type XLSXReader struct {
	name    string
	version string
}

// NewXLSXReader creates a new XLSX file reader
func NewXLSXReader() *XLSXReader {
	return &XLSXReader{
		name:    "xlsx_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *XLSXReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_formulas",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract cell formulas instead of values",
		},
		{
			Name:        "include_hidden_sheets",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include hidden worksheets",
		},
		{
			Name:        "include_empty_cells",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include empty cells in output",
		},
		{
			Name:        "max_rows_per_sheet",
			Type:        "int",
			Required:    false,
			Default:     0,
			Description: "Maximum rows to process per sheet (0 = all)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(1000000.0),
		},
		{
			Name:        "max_columns_per_sheet",
			Type:        "int",
			Required:    false,
			Default:     0,
			Description: "Maximum columns to process per sheet (0 = all)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(16384.0),
		},
		{
			Name:        "treat_first_row_as_header",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Treat first row as column headers",
		},
		{
			Name:        "extract_comments",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract cell comments",
		},
		{
			Name:        "date_format",
			Type:        "string",
			Required:    false,
			Default:     "2006-01-02",
			Description: "Date format for date cells",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *XLSXReader) ValidateConfig(config map[string]any) error {
	if maxRows, ok := config["max_rows_per_sheet"]; ok {
		if num, ok := maxRows.(float64); ok {
			if num < 0 || num > 1000000 {
				return fmt.Errorf("max_rows_per_sheet must be between 0 and 1000000")
			}
		}
	}

	if maxCols, ok := config["max_columns_per_sheet"]; ok {
		if num, ok := maxCols.(float64); ok {
			if num < 0 || num > 16384 {
				return fmt.Errorf("max_columns_per_sheet must be between 0 and 16384")
			}
		}
	}

	return nil
}

// TestConnection tests if the XLSX can be read
func (r *XLSXReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "XLSX reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_formulas":            config["extract_formulas"],
			"treat_first_row_as_header":   config["treat_first_row_as_header"],
		},
	}
}

// GetType returns the connector type
func (r *XLSXReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *XLSXReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *XLSXReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the XLSX file structure
func (r *XLSXReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	workbook, err := r.parseWorkbook(sourcePath, map[string]any{})
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to parse XLSX workbook: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "xlsx",
		Encoding: "utf-8",
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
				Name:        "column",
				Type:        "string",
				Nullable:    false,
				Description: "Column identifier (A, B, C, etc.)",
			},
			{
				Name:        "cell_value",
				Type:        "string",
				Nullable:    true,
				Description: "Cell value",
			},
			{
				Name:        "cell_type",
				Type:        "string",
				Nullable:    false,
				Description: "Cell data type (text, number, date, formula)",
			},
		},
		Metadata: map[string]any{
			"sheet_count":      len(workbook.Sheets),
			"total_rows":       workbook.TotalRows,
			"total_columns":    workbook.TotalColumns,
			"has_formulas":     workbook.HasFormulas,
			"created_date":     workbook.CreatedDate,
			"modified_date":    workbook.ModifiedDate,
			"creator":          workbook.Creator,
		},
	}

	// Sample first sheet's first few rows
	if len(workbook.Sheets) > 0 {
		sheet := workbook.Sheets[0]
		sampleData := make([]map[string]any, 0, min(len(sheet.Rows), 3))
		
		for i, row := range sheet.Rows {
			if i >= 3 {
				break
			}
			if len(row.Cells) > 0 {
				sampleData = append(sampleData, map[string]any{
					"sheet_name":  sheet.Name,
					"row_number":  i + 1,
					"column":      "A",
					"cell_value":  row.Cells[0].Value,
					"cell_type":   row.Cells[0].Type,
				})
			}
		}
		schema.SampleData = sampleData
	}

	return schema, nil
}

// EstimateSize returns size estimates for the XLSX file
func (r *XLSXReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	workbook, err := r.parseWorkbook(sourcePath, map[string]any{})
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to parse XLSX workbook: %w", err)
	}

	totalCells := int64(0)
	for _, sheet := range workbook.Sheets {
		totalCells += int64(len(sheet.Rows) * sheet.MaxColumns)
	}

	// Estimate chunks based on rows (assuming ~50 rows per chunk)
	estimatedChunks := int((int64(workbook.TotalRows) + 49) / 50)

	complexity := "low"
	if stat.Size() > 5*1024*1024 || workbook.TotalRows > 10000 { // > 5MB or > 10k rows
		complexity = "medium"
	}
	if stat.Size() > 50*1024*1024 || workbook.TotalRows > 100000 { // > 50MB or > 100k rows
		complexity = "high"
	}

	processTime := "fast"
	if stat.Size() > 10*1024*1024 || workbook.TotalRows > 50000 {
		processTime = "medium"
	}
	if stat.Size() > 100*1024*1024 || workbook.TotalRows > 500000 {
		processTime = "slow"
	}

	rowCount := int64(workbook.TotalRows)
	return core.SizeEstimate{
		RowCount:    &rowCount,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the XLSX file
func (r *XLSXReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	workbook, err := r.parseWorkbook(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XLSX workbook: %w", err)
	}

	iterator := &XLSXIterator{
		sourcePath:   sourcePath,
		config:       strategyConfig,
		workbook:     workbook,
		currentSheet: 0,
		currentRow:   0,
	}

	return iterator, nil
}

// SupportsStreaming indicates XLSX reader supports streaming
func (r *XLSXReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *XLSXReader) GetSupportedFormats() []string {
	return []string{"xlsx"}
}

// XLSXWorkbook represents a parsed XLSX workbook
type XLSXWorkbook struct {
	Sheets         []XLSXSheet
	TotalRows      int
	TotalColumns   int
	HasFormulas    bool
	CreatedDate    string
	ModifiedDate   string
	Creator        string
}

// XLSXSheet represents a worksheet
type XLSXSheet struct {
	Name       string
	Index      int
	Rows       []XLSXRow
	MaxColumns int
	Hidden     bool
}

// XLSXRow represents a row in a worksheet
type XLSXRow struct {
	Number int
	Cells  []XLSXCell
}

// XLSXCell represents a cell in a worksheet
type XLSXCell struct {
	Column   string
	Value    string
	Type     string
	Formula  string
	Comment  string
}

// parseWorkbook parses the complete XLSX workbook
func (r *XLSXReader) parseWorkbook(sourcePath string, config map[string]any) (*XLSXWorkbook, error) {
	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open XLSX file: %w", err)
	}
	defer zipReader.Close()

	// In a real implementation, would parse:
	// - app.xml for metadata
	// - workbook.xml for sheet structure
	// - sharedStrings.xml for string values
	// - sheet*.xml for worksheet data

	// Mock workbook structure
	sheets := []XLSXSheet{
		{
			Name:       "Sheet1",
			Index:      0,
			MaxColumns: 5,
			Hidden:     false,
			Rows: []XLSXRow{
				{
					Number: 1,
					Cells: []XLSXCell{
						{Column: "A", Value: "Header 1", Type: "text"},
						{Column: "B", Value: "Header 2", Type: "text"},
						{Column: "C", Value: "Header 3", Type: "text"},
					},
				},
				{
					Number: 2,
					Cells: []XLSXCell{
						{Column: "A", Value: "Row 1 Value 1", Type: "text"},
						{Column: "B", Value: "100", Type: "number"},
						{Column: "C", Value: "2024-01-15", Type: "date"},
					},
				},
				{
					Number: 3,
					Cells: []XLSXCell{
						{Column: "A", Value: "Row 2 Value 1", Type: "text"},
						{Column: "B", Value: "200", Type: "number"},
						{Column: "C", Value: "2024-01-16", Type: "date"},
					},
				},
			},
		},
	}

	workbook := &XLSXWorkbook{
		Sheets:       sheets,
		TotalRows:    3,
		TotalColumns: 3,
		HasFormulas:  false,
		CreatedDate:  time.Now().Format("2006-01-02"),
		ModifiedDate: time.Now().Format("2006-01-02"),
		Creator:      "Microsoft Excel",
	}

	return workbook, nil
}

// XLSXIterator implements ChunkIterator for XLSX files
type XLSXIterator struct {
	sourcePath   string
	config       map[string]any
	workbook     *XLSXWorkbook
	currentSheet int
	currentRow   int
}

// Next returns the next chunk of data from the XLSX
func (it *XLSXIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Find next available row
	for it.currentSheet < len(it.workbook.Sheets) {
		sheet := it.workbook.Sheets[it.currentSheet]
		
		// Skip hidden sheets if not configured to include them
		if sheet.Hidden {
			if includeHidden, ok := it.config["include_hidden_sheets"].(bool); !ok || !includeHidden {
				it.currentSheet++
				it.currentRow = 0
				continue
			}
		}

		if it.currentRow < len(sheet.Rows) {
			row := sheet.Rows[it.currentRow]
			it.currentRow++

			// Skip header row if configured
			if treatFirstAsHeader, ok := it.config["treat_first_row_as_header"].(bool); ok && treatFirstAsHeader {
				if row.Number == 1 {
					return it.Next(ctx) // Skip header and get next row
				}
			}

			// Convert row to text representation
			var cellValues []string
			for _, cell := range row.Cells {
				cellValues = append(cellValues, fmt.Sprintf("%s: %s", cell.Column, cell.Value))
			}
			content := strings.Join(cellValues, ", ")

			chunk := core.Chunk{
				Data: content,
				Metadata: core.ChunkMetadata{
					SourcePath:  it.sourcePath,
					ChunkID:     fmt.Sprintf("%s:sheet:%s:row:%d", filepath.Base(it.sourcePath), sheet.Name, row.Number),
					ChunkType:   "xlsx_row",
					SizeBytes:   int64(len(content)),
					ProcessedAt: time.Now(),
					ProcessedBy: "xlsx_reader",
					Context: map[string]string{
						"sheet_name":   sheet.Name,
						"sheet_index":  strconv.Itoa(sheet.Index),
						"row_number":   strconv.Itoa(row.Number),
						"column_count": strconv.Itoa(len(row.Cells)),
						"file_type":    "xlsx",
					},
				},
			}

			return chunk, nil
		}

		// Move to next sheet
		it.currentSheet++
		it.currentRow = 0
	}

	return core.Chunk{}, core.ErrIteratorExhausted
}

// Close releases XLSX resources
func (it *XLSXIterator) Close() error {
	// Nothing to close for XLSX iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *XLSXIterator) Reset() error {
	it.currentSheet = 0
	it.currentRow = 0
	return nil
}

// Progress returns iteration progress
func (it *XLSXIterator) Progress() float64 {
	if len(it.workbook.Sheets) == 0 {
		return 1.0
	}

	totalRows := 0
	processedRows := 0

	for i, sheet := range it.workbook.Sheets {
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

