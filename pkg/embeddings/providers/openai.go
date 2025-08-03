package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/embeddings"
)

// OpenAIProvider implements the EmbeddingProvider interface for OpenAI
type OpenAIProvider struct {
	apiKey     string
	baseURL    string
	model      string
	client     *http.Client
	dimensions int
	maxTokens  int
}

// OpenAIConfig contains configuration for OpenAI provider
type OpenAIConfig struct {
	APIKey     string        `json:"api_key"`
	BaseURL    string        `json:"base_url"`
	Model      string        `json:"model"`
	Timeout    time.Duration `json:"timeout"`
	MaxRetries int           `json:"max_retries"`
}

// OpenAI API request/response structures
type openAIEmbeddingRequest struct {
	Input          interface{} `json:"input"`
	Model          string      `json:"model"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     int         `json:"dimensions,omitempty"`
	User           string      `json:"user,omitempty"`
}

type openAIEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// NewOpenAIProvider creates a new OpenAI embedding provider
func NewOpenAIProvider(config *OpenAIConfig) (*OpenAIProvider, error) {
	if config.APIKey == "" {
		return nil, &embeddings.EmbeddingError{
			Type:    "invalid_configuration",
			Message: "OpenAI API key is required",
			Code:    "MISSING_API_KEY",
		}
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := config.Model
	if model == "" {
		model = "text-embedding-3-small"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	provider := &OpenAIProvider{
		apiKey:  config.APIKey,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		model:   model,
		client: &http.Client{
			Timeout: timeout,
		},
	}

	// Set model-specific parameters
	provider.setModelParameters()

	return provider, nil
}

// setModelParameters sets model-specific parameters
func (p *OpenAIProvider) setModelParameters() {
	switch p.model {
	case "text-embedding-3-small":
		p.dimensions = 1536
		p.maxTokens = 8192
	case "text-embedding-3-large":
		p.dimensions = 3072
		p.maxTokens = 8192
	case "text-embedding-ada-002":
		p.dimensions = 1536
		p.maxTokens = 8192
	default:
		// Default values for unknown models
		p.dimensions = 1536
		p.maxTokens = 8192
	}
}

// GenerateEmbedding creates an embedding vector for the given text
func (p *OpenAIProvider) GenerateEmbedding(ctx context.Context, text string) (*embeddings.EmbeddingVector, error) {
	if text == "" {
		return nil, &embeddings.EmbeddingError{
			Type:    "invalid_input",
			Message: "text cannot be empty",
			Code:    "EMPTY_TEXT",
		}
	}

	request := openAIEmbeddingRequest{
		Input:          text,
		Model:          p.model,
		EncodingFormat: "float",
	}

	response, err := p.makeAPIRequest(ctx, request)
	if err != nil {
		return nil, err
	}

	if len(response.Data) == 0 {
		return nil, &embeddings.EmbeddingError{
			Type:    "api_error",
			Message: "no embedding data returned from OpenAI",
			Code:    "NO_EMBEDDING_DATA",
		}
	}

	embedding := &embeddings.EmbeddingVector{
		Vector:     response.Data[0].Embedding,
		Dimensions: len(response.Data[0].Embedding),
		Model:      response.Model,
		CreatedAt:  time.Now(),
		Metadata: map[string]interface{}{
			"provider":     "openai",
			"tokens_used":  response.Usage.PromptTokens,
			"total_tokens": response.Usage.TotalTokens,
			"input_length": len(text),
		},
	}

	return embedding, nil
}

// GenerateBatchEmbeddings creates embeddings for multiple texts efficiently
func (p *OpenAIProvider) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([]*embeddings.EmbeddingVector, error) {
	if len(texts) == 0 {
		return nil, &embeddings.EmbeddingError{
			Type:    "invalid_input",
			Message: "texts array cannot be empty",
			Code:    "EMPTY_TEXTS",
		}
	}

	// Filter out empty texts
	validTexts := make([]string, 0, len(texts))
	textIndexMap := make(map[int]int) // maps result index to original index

	for i, text := range texts {
		if strings.TrimSpace(text) != "" {
			textIndexMap[len(validTexts)] = i
			validTexts = append(validTexts, text)
		}
	}

	if len(validTexts) == 0 {
		return nil, &embeddings.EmbeddingError{
			Type:    "invalid_input",
			Message: "no valid texts provided",
			Code:    "NO_VALID_TEXTS",
		}
	}

	request := openAIEmbeddingRequest{
		Input:          validTexts,
		Model:          p.model,
		EncodingFormat: "float",
	}

	response, err := p.makeAPIRequest(ctx, request)
	if err != nil {
		return nil, err
	}

	if len(response.Data) != len(validTexts) {
		return nil, &embeddings.EmbeddingError{
			Type:    "api_error",
			Message: fmt.Sprintf("expected %d embeddings, got %d", len(validTexts), len(response.Data)),
			Code:    "EMBEDDING_COUNT_MISMATCH",
		}
	}

	// Create result array with same length as input
	results := make([]*embeddings.EmbeddingVector, len(texts))
	now := time.Now()

	for i, data := range response.Data {
		originalIndex := textIndexMap[data.Index]
		results[originalIndex] = &embeddings.EmbeddingVector{
			Vector:     data.Embedding,
			Dimensions: len(data.Embedding),
			Model:      response.Model,
			CreatedAt:  now,
			Metadata: map[string]interface{}{
				"provider":     "openai",
				"batch_index":  i,
				"input_length": len(texts[originalIndex]),
				"tokens_used":  response.Usage.PromptTokens, // This is total for batch
				"total_tokens": response.Usage.TotalTokens,
			},
		}
	}

	return results, nil
}

// makeAPIRequest makes a request to the OpenAI API
func (p *OpenAIProvider) makeAPIRequest(ctx context.Context, request openAIEmbeddingRequest) (*openAIEmbeddingResponse, error) {
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, &embeddings.EmbeddingError{
			Type:    "serialization_error",
			Message: fmt.Sprintf("failed to marshal request: %v", err),
			Code:    "REQUEST_MARSHAL_ERROR",
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/embeddings", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, &embeddings.EmbeddingError{
			Type:    "request_error",
			Message: fmt.Sprintf("failed to create request: %v", err),
			Code:    "REQUEST_CREATION_ERROR",
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("User-Agent", "eAIIngest/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &embeddings.EmbeddingError{
			Type:    "network_error",
			Message: fmt.Sprintf("request failed: %v", err),
			Code:    "NETWORK_ERROR",
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &embeddings.EmbeddingError{
			Type:    "response_error",
			Message: fmt.Sprintf("failed to read response: %v", err),
			Code:    "RESPONSE_READ_ERROR",
		}
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp openAIErrorResponse
		if err := json.Unmarshal(body, &errorResp); err == nil {
			return nil, &embeddings.EmbeddingError{
				Type:    "api_error",
				Message: errorResp.Error.Message,
				Code:    errorResp.Error.Code,
				Details: map[string]interface{}{
					"status_code": resp.StatusCode,
					"error_type":  errorResp.Error.Type,
				},
			}
		}

		return nil, &embeddings.EmbeddingError{
			Type:    "api_error",
			Message: fmt.Sprintf("API request failed with status %d", resp.StatusCode),
			Code:    "API_ERROR",
			Details: map[string]interface{}{
				"status_code": resp.StatusCode,
				"response":    string(body),
			},
		}
	}

	var response openAIEmbeddingResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, &embeddings.EmbeddingError{
			Type:    "deserialization_error",
			Message: fmt.Sprintf("failed to unmarshal response: %v", err),
			Code:    "RESPONSE_UNMARSHAL_ERROR",
		}
	}

	return &response, nil
}

// GetModelInfo returns information about the embedding model
func (p *OpenAIProvider) GetModelInfo() *embeddings.ModelInfo {
	var description string
	var costPerToken float64

	switch p.model {
	case "text-embedding-3-small":
		description = "OpenAI's latest small embedding model with improved performance"
		costPerToken = 0.00002 / 1000 // $0.00002 per 1K tokens
	case "text-embedding-3-large":
		description = "OpenAI's latest large embedding model with highest performance"
		costPerToken = 0.00013 / 1000 // $0.00013 per 1K tokens
	case "text-embedding-ada-002":
		description = "OpenAI's previous generation embedding model"
		costPerToken = 0.0001 / 1000 // $0.0001 per 1K tokens
	default:
		description = "OpenAI embedding model"
		costPerToken = 0.0001 / 1000
	}

	return &embeddings.ModelInfo{
		Name:         p.model,
		Provider:     "openai",
		Dimensions:   p.dimensions,
		MaxTokens:    p.maxTokens,
		Description:  description,
		Version:      "1.0",
		ContextSize:  p.maxTokens,
		CostPerToken: costPerToken,
	}
}

// GetDimensions returns the dimensionality of the embeddings
func (p *OpenAIProvider) GetDimensions() int {
	return p.dimensions
}

// GetMaxTokens returns the maximum number of tokens supported
func (p *OpenAIProvider) GetMaxTokens() int {
	return p.maxTokens
}

// GetProviderName returns the name of the provider
func (p *OpenAIProvider) GetProviderName() string {
	return "openai"
}

// ValidateConfig validates the OpenAI configuration
func ValidateConfig(config *OpenAIConfig) error {
	if config.APIKey == "" {
		return &embeddings.EmbeddingError{
			Type:    "invalid_configuration",
			Message: "API key is required",
			Code:    "MISSING_API_KEY",
		}
	}

	if config.Model != "" {
		validModels := []string{
			"text-embedding-3-small",
			"text-embedding-3-large",
			"text-embedding-ada-002",
		}

		isValid := false
		for _, validModel := range validModels {
			if config.Model == validModel {
				isValid = true
				break
			}
		}

		if !isValid {
			return &embeddings.EmbeddingError{
				Type:    "invalid_configuration",
				Message: fmt.Sprintf("unsupported model: %s", config.Model),
				Code:    "UNSUPPORTED_MODEL",
			}
		}
	}

	if config.Timeout < 0 {
		return &embeddings.EmbeddingError{
			Type:    "invalid_configuration",
			Message: "timeout cannot be negative",
			Code:    "INVALID_TIMEOUT",
		}
	}

	return nil
}

// EstimateCost estimates the cost for processing given text
func (p *OpenAIProvider) EstimateCost(text string) float64 {
	// Rough token estimation (1 token ≈ 4 characters for English)
	tokens := len(text) / 4
	modelInfo := p.GetModelInfo()
	return float64(tokens) * modelInfo.CostPerToken
}

// EstimateBatchCost estimates the cost for processing multiple texts
func (p *OpenAIProvider) EstimateBatchCost(texts []string) float64 {
	totalTokens := 0
	for _, text := range texts {
		totalTokens += len(text) / 4
	}
	modelInfo := p.GetModelInfo()
	return float64(totalTokens) * modelInfo.CostPerToken
}
