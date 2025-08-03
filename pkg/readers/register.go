package readers

import (
	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/readers/archive"
	"github.com/jscharber/eAIIngest/pkg/readers/csv"
	"github.com/jscharber/eAIIngest/pkg/readers/email"
	"github.com/jscharber/eAIIngest/pkg/readers/html"
	"github.com/jscharber/eAIIngest/pkg/readers/image"
	"github.com/jscharber/eAIIngest/pkg/readers/json"
	"github.com/jscharber/eAIIngest/pkg/readers/markdown"
	"github.com/jscharber/eAIIngest/pkg/readers/microsoft"
	"github.com/jscharber/eAIIngest/pkg/readers/office"
	"github.com/jscharber/eAIIngest/pkg/readers/pdf"
	"github.com/jscharber/eAIIngest/pkg/readers/rtf"
	"github.com/jscharber/eAIIngest/pkg/readers/text"
	"github.com/jscharber/eAIIngest/pkg/readers/xml"
	"github.com/jscharber/eAIIngest/pkg/registry"
)

// RegisterBasicReaders registers all basic file readers with the global registry
func RegisterBasicReaders() {
	// Register text reader
	registry.GlobalRegistry.RegisterReader("text", func() core.DataSourceReader {
		return text.NewTextReader()
	})

	// Register CSV reader
	registry.GlobalRegistry.RegisterReader("csv", func() core.DataSourceReader {
		return csv.NewCSVReader()
	})

	// Register JSON reader
	registry.GlobalRegistry.RegisterReader("json", func() core.DataSourceReader {
		return json.NewJSONReader()
	})

	// Register PDF reader
	registry.GlobalRegistry.RegisterReader("pdf", func() core.DataSourceReader {
		return pdf.NewPDFReader()
	})

	// Register Office document readers
	registry.GlobalRegistry.RegisterReader("docx", func() core.DataSourceReader {
		return office.NewDOCXReader()
	})

	registry.GlobalRegistry.RegisterReader("xlsx", func() core.DataSourceReader {
		return office.NewXLSXReader()
	})

	registry.GlobalRegistry.RegisterReader("pptx", func() core.DataSourceReader {
		return office.NewPPTXReader()
	})

	// Register legacy Office document readers
	registry.GlobalRegistry.RegisterReader("doc", func() core.DataSourceReader {
		return office.NewDOCReader()
	})

	registry.GlobalRegistry.RegisterReader("xls", func() core.DataSourceReader {
		return office.NewXLSReader()
	})

	registry.GlobalRegistry.RegisterReader("ppt", func() core.DataSourceReader {
		return office.NewPPTReader()
	})

	// Register HTML reader
	registry.GlobalRegistry.RegisterReader("html", func() core.DataSourceReader {
		return html.NewHTMLReader()
	})

	// Register RTF reader
	registry.GlobalRegistry.RegisterReader("rtf", func() core.DataSourceReader {
		return rtf.NewRTFReader()
	})

	// Register Email readers
	registry.GlobalRegistry.RegisterReader("eml", func() core.DataSourceReader {
		return email.NewEMLReader()
	})

	registry.GlobalRegistry.RegisterReader("msg", func() core.DataSourceReader {
		return email.NewMSGReader()
	})

	registry.GlobalRegistry.RegisterReader("pst", func() core.DataSourceReader {
		return email.NewPSTReader()
	})

	// Register Image readers
	registry.GlobalRegistry.RegisterReader("tiff", func() core.DataSourceReader {
		return image.NewTIFFReader()
	})

	registry.GlobalRegistry.RegisterReader("png", func() core.DataSourceReader {
		return image.NewPNGReader()
	})

	registry.GlobalRegistry.RegisterReader("jpg", func() core.DataSourceReader {
		return image.NewJPGReader()
	})

	// Register XML reader
	registry.GlobalRegistry.RegisterReader("xml", func() core.DataSourceReader {
		return xml.NewXMLReader()
	})

	// Register Markdown reader
	registry.GlobalRegistry.RegisterReader("markdown", func() core.DataSourceReader {
		return markdown.NewMarkdownReader()
	})

	// Register Archive readers
	registry.GlobalRegistry.RegisterReader("zip", func() core.DataSourceReader {
		return archive.NewZIPReader()
	})

	// Register Microsoft-specific readers
	registry.GlobalRegistry.RegisterReader("teams", func() core.DataSourceReader {
		return microsoft.NewTeamsReader()
	})

	registry.GlobalRegistry.RegisterReader("one", func() core.DataSourceReader {
		return microsoft.NewOneNoteReader()
	})
}

// GetReaderForExtension returns the appropriate reader name for a file extension
func GetReaderForExtension(extension string) string {
	switch extension {
	case ".txt", ".text", ".log":
		return "text"
	case ".md", ".markdown", ".mdown", ".mkd", ".mdx":
		return "markdown"
	case ".csv", ".tsv":
		return "csv"
	case ".json", ".jsonl", ".ndjson":
		return "json"
	case ".pdf":
		return "pdf"
	case ".docx":
		return "docx"
	case ".doc":
		return "doc"
	case ".xlsx":
		return "xlsx"
	case ".xls":
		return "xls"
	case ".pptx":
		return "pptx"
	case ".ppt":
		return "ppt"
	case ".html", ".htm", ".xhtml":
		return "html"
	case ".rtf":
		return "rtf"
	case ".eml":
		return "eml"
	case ".msg":
		return "msg"
	case ".pst":
		return "pst"
	case ".tiff", ".tif":
		return "tiff"
	case ".png":
		return "png"
	case ".jpg", ".jpeg", ".jpe", ".jif", ".jfif", ".jfi":
		return "jpg"
	case ".xml", ".xsd", ".xsl", ".xslt", ".rss", ".atom", ".soap", ".wsdl":
		return "xml"
	case ".zip":
		return "zip"
	case ".one":
		return "one"
	default:
		return ""
	}
}

// GetSupportedExtensions returns all file extensions supported by the basic readers
func GetSupportedExtensions() []string {
	return []string{
		// === TEXT & DOCUMENT FORMATS ===
		// Plain text files
		".txt", ".text", ".log",
		// Markdown files
		".md", ".markdown", ".mdown", ".mkd", ".mdx",
		// Structured data
		".csv", ".tsv", ".json", ".jsonl", ".ndjson",
		".xml", ".xsd", ".xsl", ".xslt", ".rss", ".atom", ".soap", ".wsdl",
		// PDF files
		".pdf",
		// Web documents
		".html", ".htm", ".xhtml",
		// Rich Text Format
		".rtf",

		// === MICROSOFT OFFICE & PRODUCTS ===
		// Modern Office (OpenXML)
		".docx", ".xlsx", ".pptx",
		// Legacy Office (Binary)
		".doc", ".xls", ".ppt",
		// Microsoft-specific products
		".one", // OneNote

		// === EMAIL & COMMUNICATION ===
		// Email formats
		".eml", ".msg", ".pst",

		// === MEDIA & IMAGES ===
		// Image formats (OCR-enabled)
		".tiff", ".tif", ".png", ".jpg", ".jpeg", ".jpe", ".jif", ".jfif", ".jfi",

		// === ARCHIVE & COMPRESSION ===
		// Archive formats (container files)
		".zip",
	}
}

// GetDocumentFormats returns file extensions for document content
func GetDocumentFormats() []string {
	return []string{
		".txt", ".text", ".log", ".md", ".markdown", ".mdown", ".mkd", ".mdx",
		".csv", ".tsv", ".json", ".jsonl", ".ndjson", ".xml", ".xsd", ".xsl", ".xslt",
		".pdf", ".html", ".htm", ".xhtml", ".rtf",
		".docx", ".doc", ".xlsx", ".xls", ".pptx", ".ppt", ".one",
		".eml", ".msg", ".pst",
	}
}

// GetArchiveFormats returns file extensions for archive/container files
func GetArchiveFormats() []string {
	return []string{
		".zip", ".7z", ".rar", ".tar", ".gz", ".bz2", ".xz", ".lz4",
	}
}

// GetMicrosoftFormats returns Microsoft-specific file extensions
func GetMicrosoftFormats() []string {
	return []string{
		// Office Suite
		".docx", ".doc", ".xlsx", ".xls", ".pptx", ".ppt",
		// Specialized products
		".one",          // OneNote
		".vsd", ".vsdx", // Visio (planned)
		".mpp",           // Project (planned)
		".pub",           // Publisher (planned)
		".mdb", ".accdb", // Access (planned)
		".msg", ".pst", ".ost", // Outlook
	}
}

// GetCollaborationFormats returns file extensions for collaboration platforms
func GetCollaborationFormats() []string {
	return []string{
		// Microsoft Teams exports are typically JSON
		// Slack exports are typically JSON
		// Notion exports are typically JSON/CSV
		// These are handled by specific readers that detect content structure
	}
}

// ValidateBasicReaders validates all registered basic readers
func ValidateBasicReaders() error {
	readers := []string{"text", "markdown", "csv", "json", "xml", "pdf", "docx", "doc", "xlsx", "xls", "pptx", "ppt", "html", "rtf", "eml", "msg", "pst", "tiff", "png", "jpg", "zip", "teams", "one"}

	for _, name := range readers {
		if err := registry.GlobalRegistry.ValidatePlugin("reader", name); err != nil {
			return err
		}
	}

	return nil
}
