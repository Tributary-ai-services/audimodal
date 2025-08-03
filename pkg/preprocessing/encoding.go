package preprocessing

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// EncodingDetector handles character encoding detection and conversion
type EncodingDetector struct {
	supportedEncodings map[string]encoding.Encoding
}

// NewEncodingDetector creates a new encoding detector
func NewEncodingDetector() *EncodingDetector {
	return &EncodingDetector{
		supportedEncodings: map[string]encoding.Encoding{
			// Unicode
			"utf-8":    encoding.Nop, // UTF-8 is our target, no conversion needed
			"utf-16":   unicode.UTF16(unicode.BigEndian, unicode.UseBOM),
			"utf-16be": unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM),
			"utf-16le": unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM),
			"utf-32":   unicode.UTF16(unicode.BigEndian, unicode.UseBOM), // UTF32 not available, using UTF16

			// Western European
			"iso-8859-1":   charmap.ISO8859_1,
			"iso-8859-15":  charmap.ISO8859_15,
			"windows-1252": charmap.Windows1252,
			"cp1252":       charmap.Windows1252,

			// Eastern European
			"iso-8859-2":   charmap.ISO8859_2,
			"windows-1250": charmap.Windows1250,
			"cp1250":       charmap.Windows1250,

			// Cyrillic
			"iso-8859-5":   charmap.ISO8859_5,
			"windows-1251": charmap.Windows1251,
			"cp1251":       charmap.Windows1251,
			"koi8-r":       charmap.KOI8R,

			// Greek
			"iso-8859-7":   charmap.ISO8859_7,
			"windows-1253": charmap.Windows1253,

			// Turkish
			"iso-8859-9":   charmap.ISO8859_9,
			"windows-1254": charmap.Windows1254,

			// Hebrew
			"iso-8859-8":   charmap.ISO8859_8,
			"windows-1255": charmap.Windows1255,

			// Arabic
			"iso-8859-6":   charmap.ISO8859_6,
			"windows-1256": charmap.Windows1256,

			// Japanese
			"shift_jis":   japanese.ShiftJIS,
			"sjis":        japanese.ShiftJIS,
			"euc-jp":      japanese.EUCJP,
			"iso-2022-jp": japanese.ISO2022JP,

			// Korean
			"euc-kr": korean.EUCKR,
			"cp949":  korean.EUCKR,

			// Chinese
			"gb2312":  simplifiedchinese.HZGB2312,
			"gbk":     simplifiedchinese.GBK,
			"gb18030": simplifiedchinese.GB18030,
			"big5":    traditionalchinese.Big5,
		},
	}
}

// EncodingInfo contains information about detected encoding
type EncodingInfo struct {
	Name       string
	Confidence float64
	BOM        []byte
}

// DetectEncoding attempts to detect the character encoding of a file
func (ed *EncodingDetector) DetectEncoding(filePath string) (*EncodingInfo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read first 8KB for detection
	buffer := make([]byte, 8192)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	buffer = buffer[:n]

	return ed.DetectEncodingFromBytes(buffer)
}

// DetectEncodingFromBytes detects encoding from byte slice
func (ed *EncodingDetector) DetectEncodingFromBytes(data []byte) (*EncodingInfo, error) {
	if len(data) == 0 {
		return &EncodingInfo{Name: "utf-8", Confidence: 1.0}, nil
	}

	// Check for BOM (Byte Order Mark)
	if bom, encoding := ed.detectBOM(data); bom != nil {
		return &EncodingInfo{Name: encoding, Confidence: 1.0, BOM: bom}, nil
	}

	// Check if it's valid UTF-8
	if utf8.Valid(data) {
		return &EncodingInfo{Name: "utf-8", Confidence: 0.9}, nil
	}

	// Check for common patterns
	if encoding, confidence := ed.detectCommonPatterns(data); encoding != "" {
		return &EncodingInfo{Name: encoding, Confidence: confidence}, nil
	}

	// Statistical analysis for encoding detection
	encoding, confidence := ed.statisticalDetection(data)
	return &EncodingInfo{Name: encoding, Confidence: confidence}, nil
}

// detectBOM checks for Byte Order Mark
func (ed *EncodingDetector) detectBOM(data []byte) ([]byte, string) {
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xEF, 0xBB, 0xBF}) {
		return data[:3], "utf-8"
	}
	if len(data) >= 2 && bytes.Equal(data[:2], []byte{0xFF, 0xFE}) {
		return data[:2], "utf-16le"
	}
	if len(data) >= 2 && bytes.Equal(data[:2], []byte{0xFE, 0xFF}) {
		return data[:2], "utf-16be"
	}
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0xFF, 0xFE, 0x00, 0x00}) {
		return data[:4], "utf-32le"
	}
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0x00, 0x00, 0xFE, 0xFF}) {
		return data[:4], "utf-32be"
	}
	return nil, ""
}

// detectCommonPatterns looks for common encoding patterns
func (ed *EncodingDetector) detectCommonPatterns(data []byte) (string, float64) {
	// Look for HTML meta charset declaration
	htmlCharset := ed.extractHTMLCharset(data)
	if htmlCharset != "" {
		return htmlCharset, 0.8
	}

	// Look for XML encoding declaration
	xmlEncoding := ed.extractXMLEncoding(data)
	if xmlEncoding != "" {
		return xmlEncoding, 0.8
	}

	// Check for ASCII (subset of UTF-8)
	if ed.isASCII(data) {
		return "utf-8", 0.7
	}

	return "", 0.0
}

// extractHTMLCharset extracts charset from HTML meta tags
func (ed *EncodingDetector) extractHTMLCharset(data []byte) string {
	dataStr := strings.ToLower(string(data))

	// Look for <meta charset="...">
	if start := strings.Index(dataStr, "<meta charset="); start != -1 {
		start += 14
		if start < len(dataStr) {
			quote := dataStr[start]
			if quote == '"' || quote == '\'' {
				end := strings.IndexByte(dataStr[start+1:], byte(quote))
				if end != -1 {
					return dataStr[start+1 : start+1+end]
				}
			}
		}
	}

	// Look for <meta http-equiv="Content-Type" content="...charset=...">
	if start := strings.Index(dataStr, "charset="); start != -1 {
		start += 8
		end := start
		for end < len(dataStr) && (dataStr[end] != '"' && dataStr[end] != '\'' && dataStr[end] != '>' && dataStr[end] != ' ' && dataStr[end] != ';') {
			end++
		}
		if end > start {
			return dataStr[start:end]
		}
	}

	return ""
}

// extractXMLEncoding extracts encoding from XML declaration
func (ed *EncodingDetector) extractXMLEncoding(data []byte) string {
	dataStr := strings.ToLower(string(data))

	// Look for <?xml ... encoding="...">
	if start := strings.Index(dataStr, "<?xml"); start != -1 {
		xmlDecl := dataStr[start:]
		if end := strings.Index(xmlDecl, "?>"); end != -1 {
			xmlDecl = xmlDecl[:end]
			if encStart := strings.Index(xmlDecl, "encoding="); encStart != -1 {
				encStart += 9
				if encStart < len(xmlDecl) {
					quote := xmlDecl[encStart]
					if quote == '"' || quote == '\'' {
						encEnd := strings.IndexByte(xmlDecl[encStart+1:], byte(quote))
						if encEnd != -1 {
							return xmlDecl[encStart+1 : encStart+1+encEnd]
						}
					}
				}
			}
		}
	}

	return ""
}

// isASCII checks if data contains only ASCII characters
func (ed *EncodingDetector) isASCII(data []byte) bool {
	for _, b := range data {
		if b >= 128 {
			return false
		}
	}
	return true
}

// statisticalDetection uses statistical analysis to detect encoding
func (ed *EncodingDetector) statisticalDetection(data []byte) (string, float64) {
	// Count byte frequencies
	freq := make([]int, 256)
	for _, b := range data {
		freq[b]++
	}

	// Check for Windows-1252 patterns (common high-byte characters)
	windows1252Score := 0
	commonWin1252 := []byte{0x80, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89, 0x8A, 0x8B, 0x8C, 0x8E, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0x9B, 0x9C, 0x9E, 0x9F}
	for _, b := range commonWin1252 {
		windows1252Score += freq[b]
	}

	// Check for ISO-8859-1 patterns
	iso88591Score := 0
	for i := 160; i <= 255; i++ {
		iso88591Score += freq[i]
	}

	// Determine best guess based on scores
	totalBytes := len(data)
	if totalBytes == 0 {
		return "utf-8", 0.5
	}

	windows1252Ratio := float64(windows1252Score) / float64(totalBytes)
	iso88591Ratio := float64(iso88591Score) / float64(totalBytes)

	if windows1252Ratio > 0.01 {
		return "windows-1252", 0.6
	}
	if iso88591Ratio > 0.02 {
		return "iso-8859-1", 0.6
	}

	// Default to UTF-8 for unknown encodings
	return "utf-8", 0.5
}

// ConvertToUTF8 converts a file from detected encoding to UTF-8
func (ed *EncodingDetector) ConvertToUTF8(filePath string, sourceEncoding string) (string, error) {
	// If already UTF-8, return original path
	if sourceEncoding == "utf-8" {
		return filePath, nil
	}

	encoder, exists := ed.supportedEncodings[strings.ToLower(sourceEncoding)]
	if !exists {
		return "", fmt.Errorf("unsupported encoding: %s", sourceEncoding)
	}

	// Read original file
	sourceFile, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	// Create temporary UTF-8 file
	tempFile, err := os.CreateTemp(os.TempDir(), "utf8_converted_*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	// Convert encoding
	if encoder == encoding.Nop {
		// No conversion needed (already UTF-8)
		_, err = io.Copy(tempFile, sourceFile)
	} else {
		// Convert from source encoding to UTF-8
		decoder := encoder.NewDecoder()
		reader := transform.NewReader(sourceFile, decoder)
		_, err = io.Copy(tempFile, reader)
	}

	if err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to convert encoding: %w", err)
	}

	return tempFile.Name(), nil
}

// ConvertBytesToUTF8 converts byte slice from specified encoding to UTF-8
func (ed *EncodingDetector) ConvertBytesToUTF8(data []byte, sourceEncoding string) ([]byte, error) {
	if sourceEncoding == "utf-8" {
		return data, nil
	}

	encoder, exists := ed.supportedEncodings[strings.ToLower(sourceEncoding)]
	if !exists {
		return nil, fmt.Errorf("unsupported encoding: %s", sourceEncoding)
	}

	if encoder == encoding.Nop {
		return data, nil
	}

	decoder := encoder.NewDecoder()
	reader := transform.NewReader(bytes.NewReader(data), decoder)
	return io.ReadAll(reader)
}

// GetSupportedEncodings returns list of supported encodings
func (ed *EncodingDetector) GetSupportedEncodings() []string {
	encodings := make([]string, 0, len(ed.supportedEncodings))
	for encoding := range ed.supportedEncodings {
		encodings = append(encodings, encoding)
	}
	return encodings
}

// ValidateUTF8 checks if data is valid UTF-8
func (ed *EncodingDetector) ValidateUTF8(data []byte) bool {
	return utf8.Valid(data)
}

// AutoDetectAndConvert automatically detects encoding and converts to UTF-8
func (ed *EncodingDetector) AutoDetectAndConvert(filePath string) (string, *EncodingInfo, error) {
	// Detect encoding
	encodingInfo, err := ed.DetectEncoding(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to detect encoding: %w", err)
	}

	// Convert to UTF-8
	convertedPath, err := ed.ConvertToUTF8(filePath, encodingInfo.Name)
	if err != nil {
		return "", encodingInfo, fmt.Errorf("failed to convert to UTF-8: %w", err)
	}

	return convertedPath, encodingInfo, nil
}
