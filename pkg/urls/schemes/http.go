package schemes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// HTTPResolver handles http:// and https:// URLs
type HTTPResolver struct {
	client *http.Client
}

// NewHTTPResolver creates a new HTTP resolver
func NewHTTPResolver() *HTTPResolver {
	return &HTTPResolver{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Supports returns true for http and https schemes
func (h *HTTPResolver) Supports(scheme string) bool {
	scheme = strings.ToLower(scheme)
	return scheme == "http" || scheme == "https"
}

// Resolve resolves an HTTP URL to a ResolvedFile
func (h *HTTPResolver) Resolve(ctx context.Context, fileURL string) (*core.ResolvedFile, error) {
	parsed, err := url.Parse(fileURL)
	if err != nil {
		return nil, &core.URLError{
			Type:    "INVALID_HTTP_URL",
			Message: fmt.Sprintf("invalid HTTP URL: %v", err),
			URL:     fileURL,
		}
	}

	if !h.Supports(parsed.Scheme) {
		return nil, &core.URLError{
			Type:    "WRONG_SCHEME",
			Message: fmt.Sprintf("expected http:// or https:// scheme, got %s://", parsed.Scheme),
			URL:     fileURL,
			Scheme:  parsed.Scheme,
		}
	}

	req, err := http.NewRequestWithContext(ctx, "HEAD", fileURL, nil)
	if err != nil {
		return nil, &core.URLError{
			Type:    "REQUEST_CREATE_ERROR",
			Message: fmt.Sprintf("failed to create request: %v", err),
			URL:     fileURL,
		}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, &core.URLError{
			Type:    "REQUEST_FAILED",
			Message: fmt.Sprintf("failed to fetch URL: %v", err),
			URL:     fileURL,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &core.URLError{
			Type:    "HTTP_ERROR",
			Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status),
			URL:     fileURL,
		}
	}

	var size int64
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		if parsedSize, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			size = parsedSize
		}
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	filename := path.Base(parsed.Path)
	if disposition := resp.Header.Get("Content-Disposition"); disposition != "" {
		if strings.Contains(disposition, "filename=") {
			parts := strings.Split(disposition, "filename=")
			if len(parts) > 1 {
				filename = strings.Trim(parts[1], `"`)
			}
		}
	}

	var lastModified int64
	if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
		if t, err := time.Parse(time.RFC1123, lastMod); err == nil {
			lastModified = t.Unix()
		}
	}

	resolved := &core.ResolvedFile{
		URL:         fileURL,
		Scheme:      parsed.Scheme,
		Size:        size,
		ContentType: contentType,
		Metadata: map[string]string{
			"filename": filename,
			"host":     parsed.Host,
			"path":     parsed.Path,
		},
		AccessInfo: core.AccessInfo{
			RequiresAuth: false,
		},
		LastModified: lastModified,
	}

	return resolved, nil
}

// CreateIterator creates a chunk iterator for an HTTP URL
func (h *HTTPResolver) CreateIterator(ctx context.Context, fileURL string, config map[string]any) (core.ChunkIterator, error) {
	return NewHTTPIterator(fileURL, config, h.client)
}

// GetName returns the resolver name
func (h *HTTPResolver) GetName() string {
	return "http_resolver"
}

// GetVersion returns the resolver version
func (h *HTTPResolver) GetVersion() string {
	return "1.0.0"
}

// HTTPIterator implements ChunkIterator for HTTP URLs
type HTTPIterator struct {
	url    string
	config map[string]any
	client *http.Client
	resp   *http.Response
	closed bool
	read   int64
	size   int64
}

// NewHTTPIterator creates a new HTTP iterator
func NewHTTPIterator(url string, config map[string]any, client *http.Client) (core.ChunkIterator, error) {
	return &HTTPIterator{
		url:    url,
		config: config,
		client: client,
	}, nil
}

// Next returns the next chunk from the HTTP response
func (hi *HTTPIterator) Next(ctx context.Context) (core.Chunk, error) {
	if hi.closed {
		return core.Chunk{}, fmt.Errorf("iterator is closed")
	}

	if hi.resp == nil {
		req, err := http.NewRequestWithContext(ctx, "GET", hi.url, nil)
		if err != nil {
			return core.Chunk{}, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := hi.client.Do(req)
		if err != nil {
			return core.Chunk{}, fmt.Errorf("failed to fetch URL: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return core.Chunk{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		}

		hi.resp = resp

		if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
			if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
				hi.size = size
			}
		}
	}

	data, err := io.ReadAll(hi.resp.Body)
	if err != nil {
		return core.Chunk{}, fmt.Errorf("failed to read response: %w", err)
	}

	hi.read = int64(len(data))

	chunk := core.Chunk{
		Data: string(data),
		Metadata: core.ChunkMetadata{
			SourcePath:    hi.url,
			ChunkID:       "chunk_0",
			ChunkType:     "text",
			SizeBytes:     int64(len(data)),
			StartPosition: func() *int64 { v := int64(0); return &v }(),
			EndPosition:   func() *int64 { v := int64(len(data)); return &v }(),
			ProcessedAt:   time.Now(),
			ProcessedBy:   "http_iterator",
		},
	}

	hi.closed = true
	return chunk, nil
}

// Close closes the HTTP iterator
func (hi *HTTPIterator) Close() error {
	if !hi.closed {
		hi.closed = true
		if hi.resp != nil && hi.resp.Body != nil {
			return hi.resp.Body.Close()
		}
	}
	return nil
}

// Reset is not supported for HTTP streams
func (hi *HTTPIterator) Reset() error {
	return fmt.Errorf("reset not supported for HTTP streams")
}

// Progress returns the current progress
func (hi *HTTPIterator) Progress() float64 {
	if hi.size == 0 {
		return 1.0
	}
	return float64(hi.read) / float64(hi.size)
}
