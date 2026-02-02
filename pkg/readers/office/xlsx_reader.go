package office

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/audimodal/pkg/core"
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
			"extract_formulas":          config["extract_formulas"],
			"treat_first_row_as_header": config["treat_first_row_as_header"],
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
			"sheet_count":   len(workbook.Sheets),
			"total_rows":    workbook.TotalRows,
			"total_columns": workbook.TotalColumns,
			"has_formulas":  workbook.HasFormulas,
			"created_date":  workbook.CreatedDate,
			"modified_date": workbook.ModifiedDate,
			"creator":       workbook.Creator,
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
					"sheet_name": sheet.Name,
					"row_number": i + 1,
					"column":     "A",
					"cell_value": row.Cells[0].Value,
					"cell_type":  row.Cells[0].Type,
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
	Sheets       []XLSXSheet
	TotalRows    int
	TotalColumns int
	HasFormulas  bool
	CreatedDate  string
	ModifiedDate string
	Creator      string
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
	Column  string
	Value   string
	Type    string
	Formula string
	Comment string
}

// XML parsing structures for XLSX OOXML format

// xlsxWorkbookXML represents xl/workbook.xml
type xlsxWorkbookXML struct {
	XMLName xml.Name        `xml:"workbook"`
	Sheets  xlsxSheetsXML   `xml:"sheets"`
}

type xlsxSheetsXML struct {
	Sheet []xlsxSheetXML `xml:"sheet"`
}

type xlsxSheetXML struct {
	Name    string `xml:"name,attr"`
	SheetID string `xml:"sheetId,attr"`
	RelID   string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
	State   string `xml:"state,attr"`
}

// xlsxSharedStringsXML represents xl/sharedStrings.xml
type xlsxSharedStringsXML struct {
	XMLName xml.Name         `xml:"sst"`
	Count   int              `xml:"count,attr"`
	Strings []xlsxStringItem `xml:"si"`
}

type xlsxStringItem struct {
	Text  string       `xml:"t"`
	Runs  []xlsxRunXML `xml:"r"`
}

type xlsxRunXML struct {
	Text string `xml:"t"`
}

// xlsxCoreProperties represents docProps/core.xml
type xlsxCoreProperties struct {
	XMLName      xml.Name `xml:"coreProperties"`
	Title        string   `xml:"title"`
	Creator      string   `xml:"creator"`
	LastModified string   `xml:"lastModifiedBy"`
	Created      string   `xml:"created"`
	Modified     string   `xml:"modified"`
}

// readZipFileContent reads the content of a zip file entry
func (r *XLSXReader) readZipFileContent(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// parseSharedStrings parses the shared strings table
func (r *XLSXReader) parseSharedStrings(data []byte) []string {
	var sst xlsxSharedStringsXML
	if err := xml.Unmarshal(data, &sst); err != nil {
		return nil
	}

	strings := make([]string, len(sst.Strings))
	for i, si := range sst.Strings {
		if si.Text != "" {
			strings[i] = si.Text
		} else {
			// Handle rich text (multiple runs)
			var parts []string
			for _, run := range si.Runs {
				if run.Text != "" {
					parts = append(parts, run.Text)
				}
			}
			strings[i] = strings[len(parts)-1]
			if len(parts) > 0 {
				result := ""
				for _, p := range parts {
					result += p
				}
				strings[i] = result
			}
		}
	}

	return strings
}

// parseWorksheetXML parses a worksheet XML file using regex for robustness
func (r *XLSXReader) parseWorksheetXML(data []byte, sharedStrings []string) []XLSXRow {
	var rows []XLSXRow

	// Find all row elements
	rowPattern := regexp.MustCompile(`(?s)<row[^>]*r="(\d+)"[^>]*>(.*?)</row>`)
	rowMatches := rowPattern.FindAllSubmatch(data, -1)

	for _, rowMatch := range rowMatches {
		if len(rowMatch) < 3 {
			continue
		}

		rowNum, _ := strconv.Atoi(string(rowMatch[1]))
		rowContent := rowMatch[2]

		// Find all cell elements in this row
		cellPattern := regexp.MustCompile(`(?s)<c[^>]*r="([A-Z]+)\d+"[^>]*(?:t="([^"]*)")?[^>]*>(.*?)</c>`)
		cellMatches := cellPattern.FindAllSubmatch(rowContent, -1)

		var cells []XLSXCell
		maxCol := 0

		for _, cellMatch := range cellMatches {
			if len(cellMatch) < 4 {
				continue
			}

			colRef := string(cellMatch[1])
			cellType := string(cellMatch[2])
			cellContent := cellMatch[3]

			// Extract value
			var value string
			valuePattern := regexp.MustCompile(`<v>([^<]*)</v>`)
			valueMatch := valuePattern.FindSubmatch(cellContent)
			if len(valueMatch) > 1 {
				value = string(valueMatch[1])
			}

			// Extract formula if present
			var formula string
			formulaPattern := regexp.MustCompile(`<f>([^<]*)</f>`)
			formulaMatch := formulaPattern.FindSubmatch(cellContent)
			if len(formulaMatch) > 1 {
				formula = string(formulaMatch[1])
			}

			// Resolve shared string references
			cellTypeStr := "text"
			if cellType == "s" && value != "" {
				// Shared string reference
				idx, err := strconv.Atoi(value)
				if err == nil && idx < len(sharedStrings) {
					value = sharedStrings[idx]
				}
				cellTypeStr = "text"
			} else if cellType == "n" || (cellType == "" && value != "" && isNumeric(value)) {
				cellTypeStr = "number"
			} else if cellType == "b" {
				cellTypeStr = "boolean"
			} else if formula != "" {
				cellTypeStr = "formula"
			}

			// Track max column
			colNum := columnToNumber(colRef)
			if colNum > maxCol {
				maxCol = colNum
			}

			cells = append(cells, XLSXCell{
				Column:  colRef,
				Value:   value,
				Type:    cellTypeStr,
				Formula: formula,
			})
		}

		if len(cells) > 0 {
			rows = append(rows, XLSXRow{
				Number: rowNum,
				Cells:  cells,
			})
		}
	}

	// Sort rows by number
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Number < rows[j].Number
	})

	return rows
}

// columnToNumber converts column letter(s) to number (A=1, B=2, ..., Z=26, AA=27, etc.)
func columnToNumber(col string) int {
	result := 0
	for _, c := range col {
		result = result*26 + int(c-'A') + 1
	}
	return result
}

// isNumeric checks if a string represents a number
func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// parseWorkbook parses the complete XLSX workbook
func (r *XLSXReader) parseWorkbook(sourcePath string, config map[string]any) (*XLSXWorkbook, error) {
	zipReader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open XLSX file: %w", err)
	}
	defer zipReader.Close()

	// Build file map for quick lookup
	fileMap := make(map[string]*zip.File)
	for _, file := range zipReader.File {
		fileMap[file.Name] = file
	}

	workbook := &XLSXWorkbook{
		CreatedDate:  time.Now().Format("2006-01-02"),
		ModifiedDate: time.Now().Format("2006-01-02"),
	}

	// Parse core properties for metadata
	if coreFile, ok := fileMap["docProps/core.xml"]; ok {
		if data, err := r.readZipFileContent(coreFile); err == nil {
			var props xlsxCoreProperties
			if err := xml.Unmarshal(data, &props); err == nil {
				if props.Creator != "" {
					workbook.Creator = props.Creator
				}
				if props.Created != "" {
					workbook.CreatedDate = props.Created
				}
				if props.Modified != "" {
					workbook.ModifiedDate = props.Modified
				}
			}
		}
	}

	// Parse shared strings
	var sharedStrings []string
	if ssFile, ok := fileMap["xl/sharedStrings.xml"]; ok {
		if data, err := r.readZipFileContent(ssFile); err == nil {
			sharedStrings = r.parseSharedStrings(data)
		}
	}

	// Parse workbook.xml for sheet structure
	var sheetInfos []xlsxSheetXML
	if wbFile, ok := fileMap["xl/workbook.xml"]; ok {
		if data, err := r.readZipFileContent(wbFile); err == nil {
			var wb xlsxWorkbookXML
			if err := xml.Unmarshal(data, &wb); err == nil {
				sheetInfos = wb.Sheets.Sheet
			}
		}
	}

	// If no sheet info from workbook.xml, discover sheets from file names
	if len(sheetInfos) == 0 {
		sheetPattern := regexp.MustCompile(`^xl/worksheets/sheet(\d+)\.xml$`)
		for name := range fileMap {
			if matches := sheetPattern.FindStringSubmatch(name); matches != nil {
				idx, _ := strconv.Atoi(matches[1])
				sheetInfos = append(sheetInfos, xlsxSheetXML{
					Name:    fmt.Sprintf("Sheet%d", idx),
					SheetID: matches[1],
				})
			}
		}
		// Sort by sheet ID
		sort.Slice(sheetInfos, func(i, j int) bool {
			iID, _ := strconv.Atoi(sheetInfos[i].SheetID)
			jID, _ := strconv.Atoi(sheetInfos[j].SheetID)
			return iID < jID
		})
	}

	// Parse each worksheet
	totalRows := 0
	maxColumns := 0
	hasFormulas := false

	for idx, sheetInfo := range sheetInfos {
		// Determine sheet file path
		sheetPath := fmt.Sprintf("xl/worksheets/sheet%d.xml", idx+1)

		// Try to find sheet by relationship ID or by index
		sheetFile, ok := fileMap[sheetPath]
		if !ok {
			// Try without number
			sheetPath = "xl/worksheets/sheet.xml"
			sheetFile, ok = fileMap[sheetPath]
		}
		if !ok {
			continue
		}

		data, err := r.readZipFileContent(sheetFile)
		if err != nil {
			continue
		}

		rows := r.parseWorksheetXML(data, sharedStrings)

		// Calculate max columns for this sheet
		sheetMaxCols := 0
		for _, row := range rows {
			if len(row.Cells) > sheetMaxCols {
				sheetMaxCols = len(row.Cells)
			}
			for _, cell := range row.Cells {
				if cell.Formula != "" {
					hasFormulas = true
				}
			}
		}

		sheet := XLSXSheet{
			Name:       sheetInfo.Name,
			Index:      idx,
			Rows:       rows,
			MaxColumns: sheetMaxCols,
			Hidden:     sheetInfo.State == "hidden",
		}

		workbook.Sheets = append(workbook.Sheets, sheet)
		totalRows += len(rows)
		if sheetMaxCols > maxColumns {
			maxColumns = sheetMaxCols
		}
	}

	workbook.TotalRows = totalRows
	workbook.TotalColumns = maxColumns
	workbook.HasFormulas = hasFormulas

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
