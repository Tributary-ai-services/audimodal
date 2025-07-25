package urls

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/urls/schemes"
)

// URLSchemeRegistry manages URL resolvers for different schemes
type URLSchemeRegistry struct {
	mu        sync.RWMutex
	resolvers map[string]core.URLResolver
}

// NewURLSchemeRegistry creates a new registry with built-in resolvers
func NewURLSchemeRegistry() *URLSchemeRegistry {
	registry := &URLSchemeRegistry{
		resolvers: make(map[string]core.URLResolver),
	}

	// Register built-in resolvers
	registry.Register(schemes.NewFileResolver())
	registry.Register(schemes.NewHTTPResolver())

	return registry
}

// Register adds a URL resolver to the registry
func (r *URLSchemeRegistry) Register(resolver core.URLResolver) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Auto-detect supported schemes by checking common ones
	schemeList := []string{"file", "http", "https", "s3", "gs", "azure", "gdrive", "sharepoint", "box", "dropbox"}

	for _, scheme := range schemeList {
		if resolver.Supports(scheme) {
			r.resolvers[scheme] = resolver
		}
	}
}

// RegisterForScheme explicitly registers a resolver for a specific scheme
func (r *URLSchemeRegistry) RegisterForScheme(scheme string, resolver core.URLResolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolvers[scheme] = resolver
}

// Resolve finds the appropriate resolver and resolves the URL
func (r *URLSchemeRegistry) Resolve(ctx context.Context, fileURL string) (*core.ResolvedFile, error) {
	parsed, err := url.Parse(fileURL)
	if err != nil {
		return nil, &core.URLError{
			Type:    "INVALID_URL",
			Message: fmt.Sprintf("invalid URL format: %v", err),
			URL:     fileURL,
		}
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "" {
		return nil, &core.URLError{
			Type:    "NO_SCHEME",
			Message: "URL must include a scheme (file://, s3://, etc.)",
			URL:     fileURL,
		}
	}

	r.mu.RLock()
	resolver, exists := r.resolvers[scheme]
	r.mu.RUnlock()

	if !exists {
		return nil, &core.URLError{
			Type:    "UNSUPPORTED_SCHEME",
			Message: fmt.Sprintf("no resolver registered for scheme '%s'", scheme),
			URL:     fileURL,
			Scheme:  scheme,
		}
	}

	return resolver.Resolve(ctx, fileURL)
}

// CreateIterator creates an iterator for the given URL
func (r *URLSchemeRegistry) CreateIterator(ctx context.Context, fileURL string, config map[string]any) (core.ChunkIterator, error) {
	parsed, err := url.Parse(fileURL)
	if err != nil {
		return nil, &core.URLError{
			Type:    "INVALID_URL",
			Message: fmt.Sprintf("invalid URL format: %v", err),
			URL:     fileURL,
		}
	}

	scheme := strings.ToLower(parsed.Scheme)
	r.mu.RLock()
	resolver, exists := r.resolvers[scheme]
	r.mu.RUnlock()

	if !exists {
		return nil, &core.URLError{
			Type:    "UNSUPPORTED_SCHEME",
			Message: fmt.Sprintf("no resolver registered for scheme '%s'", scheme),
			URL:     fileURL,
			Scheme:  scheme,
		}
	}

	return resolver.CreateIterator(ctx, fileURL, config)
}

// GetResolver returns the resolver for a given scheme
func (r *URLSchemeRegistry) GetResolver(scheme string) (core.URLResolver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resolver, exists := r.resolvers[strings.ToLower(scheme)]
	if !exists {
		return nil, &core.URLError{
			Type:    "UNSUPPORTED_SCHEME",
			Message: fmt.Sprintf("no resolver registered for scheme '%s'", scheme),
			Scheme:  scheme,
		}
	}

	return resolver, nil
}

// ListSchemes returns all supported URL schemes
func (r *URLSchemeRegistry) ListSchemes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemes := make([]string, 0, len(r.resolvers))
	for scheme := range r.resolvers {
		schemes = append(schemes, scheme)
	}
	return schemes
}

// ResolverInfo provides information about a URL resolver
type ResolverInfo struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Schemes []string `json:"schemes"`
}

// GetResolverInfo returns information about all registered resolvers
func (r *URLSchemeRegistry) GetResolverInfo() []ResolverInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var info []ResolverInfo
	processed := make(map[string]bool)

	for _, resolver := range r.resolvers {
		name := resolver.GetName()
		if processed[name] {
			continue
		}
		processed[name] = true

		schemes := []string{}
		for s, res := range r.resolvers {
			if res.GetName() == name {
				schemes = append(schemes, s)
			}
		}

		info = append(info, ResolverInfo{
			Name:    name,
			Version: resolver.GetVersion(),
			Schemes: schemes,
		})
	}

	return info
}

// Helper functions for common URL operations

// ExtractScheme extracts the scheme from a URL string
func ExtractScheme(urlStr string) (string, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	return strings.ToLower(parsed.Scheme), nil
}

// ValidateURL checks if a URL is properly formatted
func ValidateURL(urlStr string) error {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return &core.URLError{
			Type:    "INVALID_URL",
			Message: fmt.Sprintf("invalid URL format: %v", err),
			URL:     urlStr,
		}
	}

	if parsed.Scheme == "" {
		return &core.URLError{
			Type:    "NO_SCHEME",
			Message: "URL must include a scheme",
			URL:     urlStr,
		}
	}

	return nil
}

// NormalizeURL normalizes a URL string for consistent processing
func NormalizeURL(urlStr string) (string, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	// Normalize scheme to lowercase
	parsed.Scheme = strings.ToLower(parsed.Scheme)

	// Normalize host to lowercase for applicable schemes
	if parsed.Host != "" {
		parsed.Host = strings.ToLower(parsed.Host)
	}

	return parsed.String(), nil
}
