package k3s

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"time"
)

// AudiModalClient is an HTTP client for the AudiModal API
type AudiModalClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewAudiModalClient creates a new AudiModal API client
func NewAudiModalClient(cfg *Config) *AudiModalClient {
	return &AudiModalClient{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// HealthCheck verifies the service is reachable
func (c *AudiModalClient) HealthCheck(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("creating health request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("health check request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		// If we can't decode, but got 200, it's still healthy
		return &HealthResponse{Status: "ok"}, nil
	}

	return &health, nil
}

// UploadFile uploads a file to AudiModal
func (c *AudiModalClient) UploadFile(ctx context.Context, tenantID, filename string, content []byte) (*FileResponse, error) {
	url := fmt.Sprintf("%s/api/v1/tenants/%s/files", c.baseURL, tenantID)

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return nil, fmt.Errorf("writing file content: %w", err)
	}

	// Add filename field
	if err := writer.WriteField("filename", filename); err != nil {
		return nil, fmt.Errorf("writing filename field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("creating upload request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.setAuthHeader(req)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return nil, fmt.Errorf("upload file: %w", err)
	}

	var fileResp FileResponse
	if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil {
		return nil, fmt.Errorf("decoding upload response: %w", err)
	}

	return &fileResp, nil
}

// ProcessFile triggers file processing
func (c *AudiModalClient) ProcessFile(ctx context.Context, tenantID, fileID string) error {
	url := fmt.Sprintf("%s/api/v1/tenants/%s/files/%s/process", c.baseURL, tenantID, fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("creating process request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("process request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return fmt.Errorf("process file: %w", err)
	}

	return nil
}

// GetFileStatus returns the current processing status of a file
func (c *AudiModalClient) GetFileStatus(ctx context.Context, tenantID, fileID string) (*FileStatus, error) {
	url := fmt.Sprintf("%s/api/v1/tenants/%s/files/%s", c.baseURL, tenantID, fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating status request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("status request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return nil, fmt.Errorf("get file status: %w", err)
	}

	var status FileStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decoding status response: %w", err)
	}

	return &status, nil
}

// GetViolations returns compliance violations for a file
func (c *AudiModalClient) GetViolations(ctx context.Context, tenantID, fileID string) (*ViolationsResponse, error) {
	url := fmt.Sprintf("%s/api/v1/tenants/%s/files/%s/violations", c.baseURL, tenantID, fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating violations request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("violations request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return nil, fmt.Errorf("get violations: %w", err)
	}

	var violations ViolationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&violations); err != nil {
		return nil, fmt.Errorf("decoding violations response: %w", err)
	}

	return &violations, nil
}

// GetScanResult returns the scan result for a file
func (c *AudiModalClient) GetScanResult(ctx context.Context, tenantID, fileID string) (*ScanResult, error) {
	url := fmt.Sprintf("%s/api/v1/tenants/%s/files/%s/scan", c.baseURL, tenantID, fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating scan result request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scan result request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return nil, fmt.Errorf("get scan result: %w", err)
	}

	var result ScanResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding scan result response: %w", err)
	}

	return &result, nil
}

// WaitForProcessing polls until file processing completes or timeout
func (c *AudiModalClient) WaitForProcessing(ctx context.Context, tenantID, fileID string, timeout time.Duration, pollInterval time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		status, err := c.GetFileStatus(ctx, tenantID, fileID)
		if err != nil {
			return fmt.Errorf("checking status: %w", err)
		}

		switch status.Status {
		case StatusCompleted:
			return nil
		case StatusFailed:
			return fmt.Errorf("processing failed: %s", status.Error)
		case StatusPending, StatusProcessing:
			// Continue waiting
		default:
			return fmt.Errorf("unknown status: %s", status.Status)
		}

		time.Sleep(pollInterval)
	}

	return ErrTimeout
}

// DeleteFile deletes a file from AudiModal
func (c *AudiModalClient) DeleteFile(ctx context.Context, tenantID, fileID string) error {
	url := fmt.Sprintf("%s/api/v1/tenants/%s/files/%s", c.baseURL, tenantID, fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("creating delete request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return fmt.Errorf("delete file: %w", err)
	}

	return nil
}

// setAuthHeader sets the authentication header on the request
func (c *AudiModalClient) setAuthHeader(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
}

// checkResponse checks the HTTP response for errors
func (c *AudiModalClient) checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)

	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error != "" {
		return fmt.Errorf("%s: %s", apiErr.Error, apiErr.Message)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}
}
