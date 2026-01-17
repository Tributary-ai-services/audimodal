package preprocessing

import (
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestFileDecompressor_DetectCompressionType(t *testing.T) {
	tempDir := t.TempDir()
	decompressor := NewFileDecompressor(tempDir)

	tests := []struct {
		name         string
		filename     string
		content      []byte
		expectedType string
		expectError  bool
	}{
		{
			name:         "ZIP file",
			filename:     "test.zip",
			content:      []byte("PK\x03\x04test"),
			expectedType: "zip",
		},
		{
			name:         "GZIP file",
			filename:     "test.gz",
			content:      []byte("\x1f\x8btest"),
			expectedType: "gzip",
		},
		{
			name:         "Plain text file",
			filename:     "test.txt",
			content:      []byte("Hello World"),
			expectedType: "none",
		},
		{
			name:         "TAR GZ file",
			filename:     "test.tar.gz",
			content:      []byte("\x1f\x8btest"),
			expectedType: "tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			testPath := filepath.Join(tempDir, tt.filename)
			err := os.WriteFile(testPath, tt.content, 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			compressionType, err := decompressor.DetectCompressionType(testPath)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if compressionType != tt.expectedType {
				t.Errorf("Expected compression type '%s', got '%s'", tt.expectedType, compressionType)
			}
		})
	}
}

func TestFileDecompressor_DecompressZip(t *testing.T) {
	tempDir := t.TempDir()
	decompressor := NewFileDecompressor(tempDir)

	// Create a test ZIP file
	zipPath := filepath.Join(tempDir, "test.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create ZIP file: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)

	// Add a test file to the ZIP
	fileWriter, err := zipWriter.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create file in ZIP: %v", err)
	}

	_, err = fileWriter.Write([]byte("Hello World"))
	if err != nil {
		t.Fatalf("Failed to write to ZIP file: %v", err)
	}

	err = zipWriter.Close()
	if err != nil {
		t.Fatalf("Failed to close ZIP writer: %v", err)
	}

	err = zipFile.Close()
	if err != nil {
		t.Fatalf("Failed to close ZIP file: %v", err)
	}

	// Decompress the ZIP file
	files, err := decompressor.DecompressFile(zipPath)
	if err != nil {
		t.Fatalf("Failed to decompress ZIP: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	}

	if files[0].Name != "test.txt" {
		t.Errorf("Expected file name 'test.txt', got '%s'", files[0].Name)
	}

	if files[0].Size != 11 {
		t.Errorf("Expected file size 11, got %d", files[0].Size)
	}

	// Verify extracted content
	content, err := os.ReadFile(files[0].Path)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if string(content) != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", string(content))
	}
}

func TestFileDecompressor_DecompressGzip(t *testing.T) {
	tempDir := t.TempDir()
	decompressor := NewFileDecompressor(tempDir)

	// Create a test GZIP file
	gzipPath := filepath.Join(tempDir, "test.txt.gz")
	gzipFile, err := os.Create(gzipPath)
	if err != nil {
		t.Fatalf("Failed to create GZIP file: %v", err)
	}

	gzipWriter := gzip.NewWriter(gzipFile)
	_, err = gzipWriter.Write([]byte("Hello World"))
	if err != nil {
		t.Fatalf("Failed to write to GZIP: %v", err)
	}

	err = gzipWriter.Close()
	if err != nil {
		t.Fatalf("Failed to close GZIP writer: %v", err)
	}

	err = gzipFile.Close()
	if err != nil {
		t.Fatalf("Failed to close GZIP file: %v", err)
	}

	// Decompress the GZIP file
	files, err := decompressor.DecompressFile(gzipPath)
	if err != nil {
		t.Fatalf("Failed to decompress GZIP: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	}

	if files[0].Name != "test.txt" {
		t.Errorf("Expected file name 'test.txt', got '%s'", files[0].Name)
	}

	// Verify extracted content
	content, err := os.ReadFile(files[0].Path)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if string(content) != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", string(content))
	}
}

func TestFileDecompressor_IsCompressed(t *testing.T) {
	tempDir := t.TempDir()
	decompressor := NewFileDecompressor(tempDir)

	// Test compressed file
	zipPath := filepath.Join(tempDir, "test.zip")
	err := os.WriteFile(zipPath, []byte("PK\x03\x04test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	isCompressed, err := decompressor.IsCompressed(zipPath)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !isCompressed {
		t.Error("Expected file to be detected as compressed")
	}

	// Test uncompressed file
	txtPath := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(txtPath, []byte("Hello World"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	isCompressed, err = decompressor.IsCompressed(txtPath)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if isCompressed {
		t.Error("Expected file to be detected as uncompressed")
	}
}

func TestFileDecompressor_GetSupportedFormats(t *testing.T) {
	decompressor := NewFileDecompressor("")
	formats := decompressor.GetSupportedFormats()

	expectedFormats := []string{"zip", "gzip", "gz", "tar", "tgz", "tar.gz"}

	if len(formats) != len(expectedFormats) {
		t.Errorf("Expected %d formats, got %d", len(expectedFormats), len(formats))
	}

	for _, expected := range expectedFormats {
		found := false
		for _, format := range formats {
			if format == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected format '%s' not found", expected)
		}
	}
}
