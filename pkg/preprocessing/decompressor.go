package preprocessing

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileDecompressor handles decompression of various archive formats
type FileDecompressor struct {
	tempDir string
}

// NewFileDecompressor creates a new file decompressor
func NewFileDecompressor(tempDir string) *FileDecompressor {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return &FileDecompressor{
		tempDir: tempDir,
	}
}

// DecompressedFile represents a file extracted from an archive
type DecompressedFile struct {
	Name        string
	Path        string
	Size        int64
	IsDirectory bool
	SourcePath  string // Original archive path
}

// DetectCompressionType detects the compression type of a file
func (d *FileDecompressor) DetectCompressionType(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read first few bytes to detect format
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		return "", err
	}
	header = header[:n]

	// Check file extension first (most reliable)
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".zip":
		return "zip", nil
	case ".gz", ".gzip":
		return "gzip", nil
	case ".tar":
		return "tar", nil
	case ".tgz":
		return "tar.gz", nil
	case ".tar.gz":
		return "tar.gz", nil
	}

	// Check magic bytes
	if len(header) >= 4 {
		// ZIP: PK\x03\x04 or PK\x05\x06 or PK\x07\x08
		if header[0] == 0x50 && header[1] == 0x4B {
			return "zip", nil
		}

		// GZIP: \x1f\x8b
		if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
			return "gzip", nil
		}

		// TAR: Check for POSIX tar signature at offset 257
		if len(header) >= 262 && string(header[257:262]) == "ustar" {
			return "tar", nil
		}
	}

	return "none", nil
}

// DecompressFile decompresses a file and returns list of extracted files
func (d *FileDecompressor) DecompressFile(filePath string) ([]DecompressedFile, error) {
	compressionType, err := d.DetectCompressionType(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect compression type: %w", err)
	}

	switch compressionType {
	case "zip":
		return d.decompressZip(filePath)
	case "gzip":
		return d.decompressGzip(filePath)
	case "tar":
		return d.decompressTar(filePath)
	case "tar.gz":
		return d.decompressTarGz(filePath)
	case "none":
		// Not compressed, return original file
		stat, err := os.Stat(filePath)
		if err != nil {
			return nil, err
		}
		return []DecompressedFile{
			{
				Name:        filepath.Base(filePath),
				Path:        filePath,
				Size:        stat.Size(),
				IsDirectory: stat.IsDir(),
				SourcePath:  filePath,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", compressionType)
	}
}

// decompressZip extracts ZIP archives
func (d *FileDecompressor) decompressZip(filePath string) ([]DecompressedFile, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ZIP file: %w", err)
	}
	defer reader.Close()

	var files []DecompressedFile
	extractDir := filepath.Join(d.tempDir, "zip_"+filepath.Base(filePath)+"_extract")
	
	err = os.MkdirAll(extractDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create extract directory: %w", err)
	}

	for _, file := range reader.File {
		extractPath := filepath.Join(extractDir, file.Name)
		
		// Ensure directory exists
		if file.FileInfo().IsDir() {
			err = os.MkdirAll(extractPath, file.FileInfo().Mode())
			if err != nil {
				return nil, fmt.Errorf("failed to create directory %s: %w", extractPath, err)
			}
			
			files = append(files, DecompressedFile{
				Name:        file.Name,
				Path:        extractPath,
				Size:        0,
				IsDirectory: true,
				SourcePath:  filePath,
			})
			continue
		}

		// Create parent directory if needed
		err = os.MkdirAll(filepath.Dir(extractPath), 0755)
		if err != nil {
			return nil, fmt.Errorf("failed to create parent directory: %w", err)
		}

		// Extract file
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s in archive: %w", file.Name, err)
		}

		outFile, err := os.OpenFile(extractPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
		if err != nil {
			rc.Close()
			return nil, fmt.Errorf("failed to create output file %s: %w", extractPath, err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		
		if err != nil {
			return nil, fmt.Errorf("failed to extract file %s: %w", file.Name, err)
		}

		files = append(files, DecompressedFile{
			Name:        file.Name,
			Path:        extractPath,
			Size:        int64(file.UncompressedSize64),
			IsDirectory: false,
			SourcePath:  filePath,
		})
	}

	return files, nil
}

// decompressGzip extracts GZIP files
func (d *FileDecompressor) decompressGzip(filePath string) ([]DecompressedFile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open GZIP file: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create GZIP reader: %w", err)
	}
	defer gzReader.Close()

	// Determine output filename
	baseName := filepath.Base(filePath)
	if strings.HasSuffix(baseName, ".gz") {
		baseName = baseName[:len(baseName)-3]
	} else if strings.HasSuffix(baseName, ".gzip") {
		baseName = baseName[:len(baseName)-5]
	} else {
		baseName = baseName + "_decompressed"
	}

	extractPath := filepath.Join(d.tempDir, "gzip_"+baseName)
	
	outFile, err := os.Create(extractPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	size, err := io.Copy(outFile, gzReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress GZIP: %w", err)
	}

	return []DecompressedFile{
		{
			Name:        baseName,
			Path:        extractPath,
			Size:        size,
			IsDirectory: false,
			SourcePath:  filePath,
		},
	}, nil
}

// decompressTar extracts TAR archives
func (d *FileDecompressor) decompressTar(filePath string) ([]DecompressedFile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open TAR file: %w", err)
	}
	defer file.Close()

	return d.extractTar(file, filePath)
}

// decompressTarGz extracts TAR.GZ archives
func (d *FileDecompressor) decompressTarGz(filePath string) ([]DecompressedFile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open TAR.GZ file: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create GZIP reader for TAR.GZ: %w", err)
	}
	defer gzReader.Close()

	return d.extractTar(gzReader, filePath)
}

// extractTar is a helper function to extract TAR archives
func (d *FileDecompressor) extractTar(reader io.Reader, sourcePath string) ([]DecompressedFile, error) {
	tarReader := tar.NewReader(reader)
	var files []DecompressedFile
	
	extractDir := filepath.Join(d.tempDir, "tar_"+filepath.Base(sourcePath)+"_extract")
	err := os.MkdirAll(extractDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create extract directory: %w", err)
	}

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read TAR header: %w", err)
		}

		extractPath := filepath.Join(extractDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(extractPath, os.FileMode(header.Mode))
			if err != nil {
				return nil, fmt.Errorf("failed to create directory %s: %w", extractPath, err)
			}
			
			files = append(files, DecompressedFile{
				Name:        header.Name,
				Path:        extractPath,
				Size:        0,
				IsDirectory: true,
				SourcePath:  sourcePath,
			})

		case tar.TypeReg:
			// Create parent directory if needed
			err = os.MkdirAll(filepath.Dir(extractPath), 0755)
			if err != nil {
				return nil, fmt.Errorf("failed to create parent directory: %w", err)
			}

			outFile, err := os.OpenFile(extractPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return nil, fmt.Errorf("failed to create output file %s: %w", extractPath, err)
			}

			_, err = io.Copy(outFile, tarReader)
			outFile.Close()
			
			if err != nil {
				return nil, fmt.Errorf("failed to extract file %s: %w", header.Name, err)
			}

			files = append(files, DecompressedFile{
				Name:        header.Name,
				Path:        extractPath,
				Size:        header.Size,
				IsDirectory: false,
				SourcePath:  sourcePath,
			})
		}
	}

	return files, nil
}

// IsCompressed checks if a file is compressed
func (d *FileDecompressor) IsCompressed(filePath string) (bool, error) {
	compressionType, err := d.DetectCompressionType(filePath)
	if err != nil {
		return false, err
	}
	return compressionType != "none", nil
}

// GetSupportedFormats returns supported compression formats
func (d *FileDecompressor) GetSupportedFormats() []string {
	return []string{"zip", "gzip", "gz", "tar", "tgz", "tar.gz"}
}

// Cleanup removes temporary extracted files
func (d *FileDecompressor) Cleanup(files []DecompressedFile) error {
	dirs := make(map[string]bool)
	
	// Collect unique directories to remove
	for _, file := range files {
		if !file.IsDirectory && file.SourcePath != file.Path {
			dir := filepath.Dir(file.Path)
			dirs[dir] = true
		}
	}
	
	// Remove directories
	for dir := range dirs {
		if strings.Contains(dir, "_extract") { // Safety check
			os.RemoveAll(dir)
		}
	}
	
	return nil
}