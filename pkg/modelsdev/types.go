package modelsdev

import "time"

// Database represents the complete models.dev database
type Database struct {
	Providers map[string]Provider `json:"providers"`
}

// Provider represents an AI model provider
type Provider struct {
	Models map[string]Model `json:"models"`
}

// Model represents an AI model with its specifications and capabilities
type Model struct {
	Name       string     `json:"name"`
	Family     string     `json:"family,omitempty"`
	Cost       *Cost      `json:"cost,omitempty"`
	Limit      Limit      `json:"limit"`
	Modalities Modalities `json:"modalities"`
}

// Cost represents the pricing information for a model
type Cost struct {
	Input      float64 `json:"input,omitempty"`
	Output     float64 `json:"output,omitempty"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}

// Limit represents the context and output limitations of a model
type Limit struct {
	Context int   `json:"context"`
	Output  int64 `json:"output"`
}

// Modalities represents the supported input and output types
type Modalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// CachedData represents the cached models.dev data with metadata
type CachedData struct {
	Database    Database  `json:"database"`
	LastRefresh time.Time `json:"last_refresh"`
	ETag        string    `json:"etag,omitempty"`
}
