package classification

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DocumentStructureAnalyzer analyzes document structure and organization
type DocumentStructureAnalyzer struct {
	name    string
	version string
}

// DocumentStructure represents the analyzed structure of a document
type DocumentStructure struct {
	Title       string           `json:"title,omitempty"`
	Sections    []Section        `json:"sections,omitempty"`
	Headings    []Heading        `json:"headings,omitempty"`
	Tables      []Table          `json:"tables,omitempty"`
	Lists       []List           `json:"lists,omitempty"`
	Paragraphs  []Paragraph      `json:"paragraphs,omitempty"`
	Metadata    StructureMetadata `json:"metadata"`
	Statistics  StructureStats   `json:"statistics"`
}

// Section represents a document section
type Section struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Level    int       `json:"level"`
	Content  string    `json:"content"`
	Start    int       `json:"start"`
	End      int       `json:"end"`
	Children []Section `json:"children,omitempty"`
}

// Heading represents a document heading
type Heading struct {
	Text     string `json:"text"`
	Level    int    `json:"level"`
	Position int    `json:"position"`
	Type     string `json:"type"` // markdown, html, numbered, etc.
}

// Table represents a detected table structure
type Table struct {
	ID          string     `json:"id"`
	Headers     []string   `json:"headers,omitempty"`
	Rows        [][]string `json:"rows"`
	Position    int        `json:"position"`
	RowCount    int        `json:"row_count"`
	ColumnCount int        `json:"column_count"`
	Type        string     `json:"type"` // csv, markdown, html, etc.
}

// List represents a detected list structure
type List struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"` // ordered, unordered, numbered
	Items    []ListItem `json:"items"`
	Position int        `json:"position"`
	Level    int        `json:"level"`
}

// ListItem represents an item in a list
type ListItem struct {
	Text     string `json:"text"`
	Level    int    `json:"level"`
	Position int    `json:"position"`
	SubItems []ListItem `json:"sub_items,omitempty"`
}

// Paragraph represents a document paragraph
type Paragraph struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Position int    `json:"position"`
	Length   int    `json:"length"`
	Type     string `json:"type"` // normal, quote, code, etc.
}

// StructureMetadata contains metadata about the document structure
type StructureMetadata struct {
	HasTitle      bool   `json:"has_title"`
	HasTOC        bool   `json:"has_toc"`
	HasHeaders    bool   `json:"has_headers"`
	HasFooters    bool   `json:"has_footers"`
	HasTables     bool   `json:"has_tables"`
	HasLists      bool   `json:"has_lists"`
	HasImages     bool   `json:"has_images"`
	HasLinks      bool   `json:"has_links"`
	HasCodeBlocks bool   `json:"has_code_blocks"`
	HasQuotes     bool   `json:"has_quotes"`
	DocumentType  string `json:"document_type"`
	Language      string `json:"language,omitempty"`
}

// StructureStats contains statistics about the document structure
type StructureStats struct {
	TotalSections    int     `json:"total_sections"`
	TotalHeadings    int     `json:"total_headings"`
	TotalParagraphs  int     `json:"total_paragraphs"`
	TotalTables      int     `json:"total_tables"`
	TotalLists       int     `json:"total_lists"`
	MaxHeadingLevel  int     `json:"max_heading_level"`
	AvgParagraphLen  float64 `json:"avg_paragraph_length"`
	WordCount        int     `json:"word_count"`
	CharacterCount   int     `json:"character_count"`
	LineCount        int     `json:"line_count"`
	ReadingTime      int     `json:"reading_time_minutes"`
}

// NewDocumentStructureAnalyzer creates a new document structure analyzer
func NewDocumentStructureAnalyzer() *DocumentStructureAnalyzer {
	return &DocumentStructureAnalyzer{
		name:    "document-structure-analyzer",
		version: "1.0.0",
	}
}

// AnalyzeStructure analyzes the structure of a document
func (d *DocumentStructureAnalyzer) AnalyzeStructure(ctx context.Context, content string, contentType ContentType) (*DocumentStructure, error) {
	if len(content) == 0 {
		return &DocumentStructure{}, nil
	}
	
	structure := &DocumentStructure{
		Sections:   []Section{},
		Headings:   []Heading{},
		Tables:     []Table{},
		Lists:      []List{},
		Paragraphs: []Paragraph{},
		Metadata:   StructureMetadata{},
		Statistics: StructureStats{},
	}
	
	// Analyze based on content type
	switch contentType {
	case ContentTypeWeb:
		d.analyzeHTMLStructure(content, structure)
	case ContentTypeDocument:
		d.analyzeTextStructure(content, structure)
	case ContentTypeCode:
		d.analyzeCodeStructure(content, structure)
	case ContentTypeEmail:
		d.analyzeEmailStructure(content, structure)
	default:
		d.analyzeTextStructure(content, structure)
	}
	
	// Calculate statistics
	d.calculateStatistics(content, structure)
	
	// Analyze metadata
	d.analyzeMetadata(content, structure)
	
	return structure, nil
}

// analyzeHTMLStructure analyzes HTML document structure
func (d *DocumentStructureAnalyzer) analyzeHTMLStructure(content string, structure *DocumentStructure) {
	// Extract title
	if titleMatch := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`).FindStringSubmatch(content); len(titleMatch) > 1 {
		structure.Title = strings.TrimSpace(titleMatch[1])
	}
	
	// Extract headings (h1-h6)
	headingRegex := regexp.MustCompile(`(?i)<h([1-6])[^>]*>(.*?)</h[1-6]>`)
	headingMatches := headingRegex.FindAllStringSubmatch(content, -1)
	
	for _, match := range headingMatches {
		if len(match) >= 3 {
			level, _ := strconv.Atoi(match[1])
			text := d.stripHTMLTags(match[2])
			position := strings.Index(content, match[0])
			
			structure.Headings = append(structure.Headings, Heading{
				Text:     strings.TrimSpace(text),
				Level:    level,
				Position: position,
				Type:     "html",
			})
		}
	}
	
	// Extract tables
	d.extractHTMLTables(content, structure)
	
	// Extract lists
	d.extractHTMLLists(content, structure)
	
	// Extract paragraphs
	d.extractHTMLParagraphs(content, structure)
	
	// Build sections from headings
	d.buildSectionsFromHeadings(content, structure)
}

// analyzeTextStructure analyzes plain text or markdown structure
func (d *DocumentStructureAnalyzer) analyzeTextStructure(content string, structure *DocumentStructure) {
	lines := strings.Split(content, "\n")
	
	// Detect markdown headings
	d.extractMarkdownHeadings(lines, structure)
	
	// Detect text-based tables
	d.extractTextTables(lines, structure)
	
	// Detect lists
	d.extractTextLists(lines, structure)
	
	// Extract paragraphs
	d.extractTextParagraphs(content, structure)
	
	// Try to identify title (first non-empty line or largest heading)
	d.identifyTitle(lines, structure)
	
	// Build sections
	d.buildSectionsFromHeadings(content, structure)
}

// analyzeCodeStructure analyzes code structure
func (d *DocumentStructureAnalyzer) analyzeCodeStructure(content string, structure *DocumentStructure) {
	lines := strings.Split(content, "\n")
	
	// Extract code blocks and functions as sections
	d.extractCodeSections(lines, structure)
	
	// Extract comments as documentation
	d.extractCodeComments(lines, structure)
	
	// Analyze indentation structure
	d.analyzeCodeIndentation(lines, structure)
}

// analyzeEmailStructure analyzes email structure
func (d *DocumentStructureAnalyzer) analyzeEmailStructure(content string, structure *DocumentStructure) {
	lines := strings.Split(content, "\n")
	
	// Extract email headers
	d.extractEmailHeaders(lines, structure)
	
	// Extract email body
	d.extractEmailBody(lines, structure)
	
	// Extract quoted text
	d.extractEmailQuotes(lines, structure)
}

// extractMarkdownHeadings extracts markdown-style headings
func (d *DocumentStructureAnalyzer) extractMarkdownHeadings(lines []string, structure *DocumentStructure) {
	position := 0
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// ATX style headings (# ## ###)
		if match := regexp.MustCompile(`^(#{1,6})\s+(.+)$`).FindStringSubmatch(trimmed); len(match) > 2 {
			level := len(match[1])
			text := strings.TrimSpace(match[2])
			
			structure.Headings = append(structure.Headings, Heading{
				Text:     text,
				Level:    level,
				Position: position,
				Type:     "markdown",
			})
		}
		
		// Setext style headings (underlined with = or -)
		if len(structure.Headings) > 0 && len(trimmed) > 0 {
			if regexp.MustCompile(`^=+$`).MatchString(trimmed) {
				// This is an h1 underline, update the previous heading
				lastIdx := len(structure.Headings) - 1
				structure.Headings[lastIdx].Level = 1
				structure.Headings[lastIdx].Type = "setext"
			} else if regexp.MustCompile(`^-+$`).MatchString(trimmed) {
				// This is an h2 underline
				lastIdx := len(structure.Headings) - 1
				structure.Headings[lastIdx].Level = 2
				structure.Headings[lastIdx].Type = "setext"
			}
		}
		
		position += len(line) + 1 // +1 for newline
	}
}

// extractHTMLTables extracts tables from HTML
func (d *DocumentStructureAnalyzer) extractHTMLTables(content string, structure *DocumentStructure) {
	tableRegex := regexp.MustCompile(`(?is)<table[^>]*>(.*?)</table>`)
	tableMatches := tableRegex.FindAllStringSubmatch(content, -1)
	
	for i, match := range tableMatches {
		if len(match) > 1 {
			tableContent := match[1]
			position := strings.Index(content, match[0])
			
			table := Table{
				ID:       "table_" + strconv.Itoa(i),
				Position: position,
				Type:     "html",
			}
			
			// Extract headers
			headerRegex := regexp.MustCompile(`(?is)<th[^>]*>(.*?)</th>`)
			headerMatches := headerRegex.FindAllStringSubmatch(tableContent, -1)
			for _, header := range headerMatches {
				if len(header) > 1 {
					table.Headers = append(table.Headers, d.stripHTMLTags(header[1]))
				}
			}
			
			// Extract rows
			rowRegex := regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
			rowMatches := rowRegex.FindAllStringSubmatch(tableContent, -1)
			
			for _, rowMatch := range rowMatches {
				if len(rowMatch) > 1 {
					cellRegex := regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
					cellMatches := cellRegex.FindAllStringSubmatch(rowMatch[1], -1)
					
					row := []string{}
					for _, cell := range cellMatches {
						if len(cell) > 1 {
							row = append(row, strings.TrimSpace(d.stripHTMLTags(cell[1])))
						}
					}
					
					if len(row) > 0 {
						table.Rows = append(table.Rows, row)
					}
				}
			}
			
			table.RowCount = len(table.Rows)
			if len(table.Rows) > 0 {
				table.ColumnCount = len(table.Rows[0])
			}
			
			structure.Tables = append(structure.Tables, table)
		}
	}
}

// extractTextTables extracts tables from plain text (CSV-like or aligned columns)
func (d *DocumentStructureAnalyzer) extractTextTables(lines []string, structure *DocumentStructure) {
	var currentTable *Table
	var tableLines []string
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Check for table patterns
		if d.looksLikeTableRow(trimmed) {
			if currentTable == nil {
				currentTable = &Table{
					ID:       "table_" + strconv.Itoa(len(structure.Tables)),
					Position: i,
					Type:     "text",
				}
				tableLines = []string{}
			}
			tableLines = append(tableLines, trimmed)
		} else {
			// End of table
			if currentTable != nil && len(tableLines) > 1 {
				d.parseTextTable(tableLines, currentTable)
				structure.Tables = append(structure.Tables, *currentTable)
			}
			currentTable = nil
			tableLines = nil
		}
	}
	
	// Handle table at end of document
	if currentTable != nil && len(tableLines) > 1 {
		d.parseTextTable(tableLines, currentTable)
		structure.Tables = append(structure.Tables, *currentTable)
	}
}

// looksLikeTableRow checks if a line looks like a table row
func (d *DocumentStructureAnalyzer) looksLikeTableRow(line string) bool {
	// Check for common table separators
	separators := []string{"|", "\t", ",", ";"}
	
	for _, sep := range separators {
		if strings.Count(line, sep) >= 2 {
			return true
		}
	}
	
	// Check for aligned columns (multiple spaces)
	if regexp.MustCompile(`\S+\s{2,}\S+`).MatchString(line) {
		return true
	}
	
	return false
}

// parseTextTable parses text table lines into a Table structure
func (d *DocumentStructureAnalyzer) parseTextTable(lines []string, table *Table) {
	if len(lines) == 0 {
		return
	}
	
	// Detect separator
	separator := d.detectTableSeparator(lines[0])
	
	for i, line := range lines {
		var cells []string
		
		if separator == "spaces" {
			// Split by multiple spaces
			cells = regexp.MustCompile(`\s{2,}`).Split(strings.TrimSpace(line), -1)
		} else {
			cells = strings.Split(line, separator)
		}
		
		// Clean cells
		for j, cell := range cells {
			cells[j] = strings.TrimSpace(cell)
		}
		
		// First row might be headers
		if i == 0 && d.looksLikeHeaders(cells) {
			table.Headers = cells
		} else {
			table.Rows = append(table.Rows, cells)
		}
	}
	
	table.RowCount = len(table.Rows)
	if len(table.Rows) > 0 {
		table.ColumnCount = len(table.Rows[0])
	}
}

// detectTableSeparator detects the separator used in a table
func (d *DocumentStructureAnalyzer) detectTableSeparator(line string) string {
	separators := map[string]int{
		"|": strings.Count(line, "|"),
		"\t": strings.Count(line, "\t"),
		",": strings.Count(line, ","),
		";": strings.Count(line, ";"),
	}
	
	maxCount := 0
	bestSep := "spaces"
	
	for sep, count := range separators {
		if count > maxCount {
			maxCount = count
			bestSep = sep
		}
	}
	
	if maxCount < 2 {
		return "spaces"
	}
	
	return bestSep
}

// looksLikeHeaders checks if cells look like table headers
func (d *DocumentStructureAnalyzer) looksLikeHeaders(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	
	// Headers often have:
	// - No numbers
	// - Consistent capitalization
	// - Shorter text
	
	hasNumbers := false
	for _, cell := range cells {
		if regexp.MustCompile(`\d+`).MatchString(cell) {
			hasNumbers = true
			break
		}
	}
	
	return !hasNumbers && len(cells[0]) < 50
}

// extractHTMLLists extracts lists from HTML
func (d *DocumentStructureAnalyzer) extractHTMLLists(content string, structure *DocumentStructure) {
	// Extract ordered lists
	d.extractHTMLListType(content, structure, "ol", "ordered")
	
	// Extract unordered lists
	d.extractHTMLListType(content, structure, "ul", "unordered")
}

// extractHTMLListType extracts a specific type of HTML list
func (d *DocumentStructureAnalyzer) extractHTMLListType(content string, structure *DocumentStructure, tagName, listType string) {
	listRegex := regexp.MustCompile(`(?is)<` + tagName + `[^>]*>(.*?)</` + tagName + `>`)
	listMatches := listRegex.FindAllStringSubmatch(content, -1)
	
	for i, match := range listMatches {
		if len(match) > 1 {
			listContent := match[1]
			position := strings.Index(content, match[0])
			
			list := List{
				ID:       listType + "_list_" + strconv.Itoa(i),
				Type:     listType,
				Position: position,
				Level:    1,
			}
			
			// Extract list items
			itemRegex := regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`)
			itemMatches := itemRegex.FindAllStringSubmatch(listContent, -1)
			
			for _, item := range itemMatches {
				if len(item) > 1 {
					text := d.stripHTMLTags(item[1])
					list.Items = append(list.Items, ListItem{
						Text:     strings.TrimSpace(text),
						Level:    1,
						Position: strings.Index(listContent, item[0]),
					})
				}
			}
			
			if len(list.Items) > 0 {
				structure.Lists = append(structure.Lists, list)
			}
		}
	}
}

// extractTextLists extracts lists from plain text
func (d *DocumentStructureAnalyzer) extractTextLists(lines []string, structure *DocumentStructure) {
	var currentList *List
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		if match := d.getListItemMatch(trimmed); match != nil {
			if currentList == nil || currentList.Type != match.Type {
				// Start new list
				if currentList != nil && len(currentList.Items) > 0 {
					structure.Lists = append(structure.Lists, *currentList)
				}
				
				currentList = &List{
					ID:       match.Type + "_list_" + strconv.Itoa(len(structure.Lists)),
					Type:     match.Type,
					Position: i,
					Level:    1,
				}
			}
			
			currentList.Items = append(currentList.Items, ListItem{
				Text:     match.Text,
				Level:    match.Level,
				Position: i,
			})
		} else {
			// End current list if we have one
			if currentList != nil && len(currentList.Items) > 0 {
				structure.Lists = append(structure.Lists, *currentList)
				currentList = nil
			}
		}
	}
	
	// Handle list at end
	if currentList != nil && len(currentList.Items) > 0 {
		structure.Lists = append(structure.Lists, *currentList)
	}
}

// listItemMatch represents a matched list item
type listItemMatch struct {
	Type  string
	Text  string
	Level int
}

// getListItemMatch checks if a line is a list item and returns match info
func (d *DocumentStructureAnalyzer) getListItemMatch(line string) *listItemMatch {
	// Numbered list (1. 2. 1) 2))
	if match := regexp.MustCompile(`^(\s*)(\d+)[\.\)]\s+(.+)$`).FindStringSubmatch(line); len(match) > 3 {
		level := len(match[1])/2 + 1
		return &listItemMatch{
			Type:  "ordered",
			Text:  match[3],
			Level: level,
		}
	}
	
	// Bullet list (- * +)
	if match := regexp.MustCompile(`^(\s*)[-\*\+]\s+(.+)$`).FindStringSubmatch(line); len(match) > 2 {
		level := len(match[1])/2 + 1
		return &listItemMatch{
			Type:  "unordered",
			Text:  match[2],
			Level: level,
		}
	}
	
	return nil
}

// extractHTMLParagraphs extracts paragraphs from HTML
func (d *DocumentStructureAnalyzer) extractHTMLParagraphs(content string, structure *DocumentStructure) {
	paragraphRegex := regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	paragraphMatches := paragraphRegex.FindAllStringSubmatch(content, -1)
	
	for i, match := range paragraphMatches {
		if len(match) > 1 {
			text := d.stripHTMLTags(match[1])
			text = strings.TrimSpace(text)
			
			if len(text) > 0 {
				position := strings.Index(content, match[0])
				structure.Paragraphs = append(structure.Paragraphs, Paragraph{
					ID:       "paragraph_" + strconv.Itoa(i),
					Text:     text,
					Position: position,
					Length:   len(text),
					Type:     "normal",
				})
			}
		}
	}
}

// extractTextParagraphs extracts paragraphs from plain text
func (d *DocumentStructureAnalyzer) extractTextParagraphs(content string, structure *DocumentStructure) {
	// Split by double newlines to get paragraphs
	paragraphs := regexp.MustCompile(`\n\s*\n`).Split(content, -1)
	position := 0
	
	for i, para := range paragraphs {
		para = strings.TrimSpace(para)
		if len(para) > 0 {
			paragraphType := "normal"
			
			// Detect special paragraph types
			if strings.HasPrefix(para, ">") {
				paragraphType = "quote"
			} else if strings.HasPrefix(para, "```") || strings.HasPrefix(para, "    ") {
				paragraphType = "code"
			}
			
			structure.Paragraphs = append(structure.Paragraphs, Paragraph{
				ID:       "paragraph_" + strconv.Itoa(i),
				Text:     para,
				Position: position,
				Length:   len(para),
				Type:     paragraphType,
			})
		}
		
		position += len(para) + 2 // +2 for double newline
	}
}

// buildSectionsFromHeadings builds hierarchical sections from headings
func (d *DocumentStructureAnalyzer) buildSectionsFromHeadings(content string, structure *DocumentStructure) {
	if len(structure.Headings) == 0 {
		return
	}
	
	// Sort headings by position
	sort.Slice(structure.Headings, func(i, j int) bool {
		return structure.Headings[i].Position < structure.Headings[j].Position
	})
	
	var sections []Section
	contentLength := len(content)
	
	for i, heading := range structure.Headings {
		section := Section{
			ID:    "section_" + strconv.Itoa(i),
			Title: heading.Text,
			Level: heading.Level,
			Start: heading.Position,
		}
		
		// Determine section end
		if i < len(structure.Headings)-1 {
			section.End = structure.Headings[i+1].Position
		} else {
			section.End = contentLength
		}
		
		// Extract section content
		if section.End > section.Start {
			section.Content = strings.TrimSpace(content[section.Start:section.End])
		}
		
		sections = append(sections, section)
	}
	
	// Build hierarchy
	structure.Sections = d.buildSectionHierarchy(sections)
}

// buildSectionHierarchy builds hierarchical section structure
func (d *DocumentStructureAnalyzer) buildSectionHierarchy(sections []Section) []Section {
	if len(sections) == 0 {
		return sections
	}
	
	var result []Section
	var stack []Section
	
	for _, section := range sections {
		// Pop stack until we find a parent at a higher level
		for len(stack) > 0 && stack[len(stack)-1].Level >= section.Level {
			stack = stack[:len(stack)-1]
		}
		
		if len(stack) == 0 {
			// Top-level section
			result = append(result, section)
			stack = append(stack, section)
		} else {
			// Child section - add to parent
			parentIdx := len(stack) - 1
			if len(result) > 0 {
				d.addChildSection(&result[len(result)-1], section, stack[parentIdx].Level)
			}
			stack = append(stack, section)
		}
	}
	
	return result
}

// addChildSection recursively adds a child section to the appropriate parent
func (d *DocumentStructureAnalyzer) addChildSection(parent *Section, child Section, targetLevel int) {
	if parent.Level == targetLevel {
		parent.Children = append(parent.Children, child)
	} else if len(parent.Children) > 0 {
		d.addChildSection(&parent.Children[len(parent.Children)-1], child, targetLevel)
	}
}

// stripHTMLTags removes HTML tags from text
func (d *DocumentStructureAnalyzer) stripHTMLTags(text string) string {
	tagRegex := regexp.MustCompile(`<[^>]*>`)
	return tagRegex.ReplaceAllString(text, "")
}

// calculateStatistics calculates document statistics
func (d *DocumentStructureAnalyzer) calculateStatistics(content string, structure *DocumentStructure) {
	structure.Statistics.CharacterCount = len(content)
	structure.Statistics.LineCount = len(strings.Split(content, "\n"))
	structure.Statistics.WordCount = len(strings.Fields(content))
	structure.Statistics.TotalSections = len(structure.Sections)
	structure.Statistics.TotalHeadings = len(structure.Headings)
	structure.Statistics.TotalParagraphs = len(structure.Paragraphs)
	structure.Statistics.TotalTables = len(structure.Tables)
	structure.Statistics.TotalLists = len(structure.Lists)
	
	// Calculate max heading level
	maxLevel := 0
	for _, heading := range structure.Headings {
		if heading.Level > maxLevel {
			maxLevel = heading.Level
		}
	}
	structure.Statistics.MaxHeadingLevel = maxLevel
	
	// Calculate average paragraph length
	if len(structure.Paragraphs) > 0 {
		totalLength := 0
		for _, para := range structure.Paragraphs {
			totalLength += para.Length
		}
		structure.Statistics.AvgParagraphLen = float64(totalLength) / float64(len(structure.Paragraphs))
	}
	
	// Estimate reading time (average 200 words per minute)
	if structure.Statistics.WordCount > 0 {
		structure.Statistics.ReadingTime = (structure.Statistics.WordCount + 199) / 200
	}
}

// analyzeMetadata analyzes document metadata
func (d *DocumentStructureAnalyzer) analyzeMetadata(content string, structure *DocumentStructure) {
	structure.Metadata.HasTitle = structure.Title != ""
	structure.Metadata.HasHeaders = len(structure.Headings) > 0
	structure.Metadata.HasTables = len(structure.Tables) > 0
	structure.Metadata.HasLists = len(structure.Lists) > 0
	
	// Check for various content features
	lowerContent := strings.ToLower(content)
	structure.Metadata.HasImages = regexp.MustCompile(`(?i)<img|!\[.*\]\(.*\)`).MatchString(content)
	structure.Metadata.HasLinks = regexp.MustCompile(`(?i)<a\s+[^>]*href|https?://|\[.*\]\(.*\)`).MatchString(content)
	structure.Metadata.HasCodeBlocks = regexp.MustCompile("```|<code>|<pre>").MatchString(content)
	structure.Metadata.HasQuotes = strings.Contains(content, ">") || regexp.MustCompile(`<blockquote>`).MatchString(content)
	
	// Check for table of contents
	structure.Metadata.HasTOC = regexp.MustCompile(`(?i)(table of contents|toc|contents)`).MatchString(lowerContent)
	
	// Basic document type detection
	if len(structure.Headings) > 3 && len(structure.Sections) > 2 {
		structure.Metadata.DocumentType = "structured_document"
	} else if len(structure.Tables) > 0 {
		structure.Metadata.DocumentType = "data_document"
	} else if len(structure.Lists) > len(structure.Paragraphs) {
		structure.Metadata.DocumentType = "list_document"
	} else {
		structure.Metadata.DocumentType = "simple_document"
	}
}

// extractCodeSections extracts code sections (functions, classes, etc.)
func (d *DocumentStructureAnalyzer) extractCodeSections(lines []string, structure *DocumentStructure) {
	// This would analyze code structure - functions, classes, etc.
	// Implementation depends on programming language
}

// extractCodeComments extracts comments from code
func (d *DocumentStructureAnalyzer) extractCodeComments(lines []string, structure *DocumentStructure) {
	// Extract code comments as documentation sections
}

// analyzeCodeIndentation analyzes code indentation structure
func (d *DocumentStructureAnalyzer) analyzeCodeIndentation(lines []string, structure *DocumentStructure) {
	// Analyze indentation patterns to understand code structure
}

// extractEmailHeaders extracts email headers
func (d *DocumentStructureAnalyzer) extractEmailHeaders(lines []string, structure *DocumentStructure) {
	// Extract From, To, Subject, Date, etc.
}

// extractEmailBody extracts email body content
func (d *DocumentStructureAnalyzer) extractEmailBody(lines []string, structure *DocumentStructure) {
	// Extract main email content
}

// extractEmailQuotes extracts quoted text from emails
func (d *DocumentStructureAnalyzer) extractEmailQuotes(lines []string, structure *DocumentStructure) {
	// Extract quoted/replied text (lines starting with >)
}

// identifyTitle attempts to identify document title
func (d *DocumentStructureAnalyzer) identifyTitle(lines []string, structure *DocumentStructure) {
	if len(lines) == 0 {
		return
	}
	
	// If we have headings, use the first level 1 heading
	for _, heading := range structure.Headings {
		if heading.Level == 1 {
			structure.Title = heading.Text
			return
		}
	}
	
	// Otherwise, use the first non-empty line if it's short enough
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 && len(trimmed) < 100 && !strings.Contains(trimmed, "\n") {
			structure.Title = trimmed
			return
		}
	}
}