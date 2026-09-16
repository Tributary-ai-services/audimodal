package processors

import "testing"

func TestProcessingPath(t *testing.T) {
	tests := []struct {
		name, path, url, want string
	}{
		{"multipart upload stores bare key in path", "3f2a/notes.txt", "s3://tenant-abc/3f2a/notes.txt", "s3://tenant-abc/3f2a/notes.txt"},
		{"URL registration stores s3 URL in path", "s3://tenant-abc/3f2a/notes.txt", "s3://tenant-abc/3f2a/notes.txt", "s3://tenant-abc/3f2a/notes.txt"},
		{"local storage fallback keeps local path", "/app/data/storage/t/f_notes.txt", "file:///app/data/storage/t/f_notes.txt", "/app/data/storage/t/f_notes.txt"},
		{"no URL keeps path", "/tmp/notes.txt", "", "/tmp/notes.txt"},
		{"non-s3 remote URL keeps path", "notes.txt", "https://example.com/notes.txt", "notes.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProcessingPath(tt.path, tt.url); got != tt.want {
				t.Errorf("ProcessingPath(%q, %q) = %q, want %q", tt.path, tt.url, got, tt.want)
			}
		})
	}
}
