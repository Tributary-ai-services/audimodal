package processors

import "strings"

// ProcessingPath returns the location the processing pipeline should read a
// file from.
//
// Multipart uploads store the bare object key in files.path (for example
// "<uuid>/report.txt") and the full "s3://bucket/key" in files.url. The
// pipeline only downloads paths that start with "s3://" and treats anything
// else as a local file, so passing the bare key made every uploaded non-PDF
// fail with "failed to get file info: stat ...". Files registered by URL
// already store the s3:// URL in path and are returned unchanged, as are
// local-storage uploads (whose url is file://).
func ProcessingPath(path, url string) string {
	if !strings.HasPrefix(path, "s3://") && strings.HasPrefix(url, "s3://") {
		return url
	}
	return path
}
