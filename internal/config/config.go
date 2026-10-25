// Package config loads and validates runtime configuration from the environment.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Transport selects how the MCP server talks to its client.
type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
)

// Config is the fully validated runtime configuration.
type Config struct {
	// BaseURL is the New API origin, e.g. http://127.0.0.1:3001.
	// It must not carry a trailing slash or an /api suffix.
	BaseURL string
	// AccessToken is a New API personal access token (not a relay/API key).
	AccessToken string
	// UserID is the numeric New API user id sent as the New-Api-User header.
	UserID string
	// Timeout bounds a single upstream request.
	Timeout time.Duration
	// ReadOnly rejects every mutating tool call.
	ReadOnly bool

	// QuotaPerUnit converts New API's internal quota integers to currency units.
	// New API ships 500000 quota per $1 (common/constants.go: QuotaPerUnit).
	QuotaPerUnit float64

	Transport Transport
	HTTPAddr  string
	HTTPToken string

	// MetricsAddr is empty by default: the metrics listener is opt-in so the
	// process never opens a port the operator did not ask for.
	MetricsAddr string

	LogLevel  string
	LogFormat string
}

// Load reads configuration from the environment and validates it.
func Load() (*Config, error) {
	cfg := &Config{
		BaseURL:      strings.TrimSpace(os.Getenv("NEWAPI_BASE_URL")),
		AccessToken:  strings.TrimSpace(os.Getenv("NEWAPI_ACCESS_TOKEN")),
		UserID:       envOr("NEWAPI_USER_ID", "1"),
		Timeout:      durationOr("NEWAPI_TIMEOUT", 60*time.Second),
		ReadOnly:     boolOr("NEWAPI_READ_ONLY", false),
		QuotaPerUnit: floatOr("NEWAPI_QUOTA_PER_UNIT", 500000),
		Transport:    Transport(strings.ToLower(envOr("MCP_TRANSPORT", "stdio"))),
		HTTPAddr:     envOr("MCP_HTTP_ADDR", "127.0.0.1:8080"),
		HTTPToken:    os.Getenv("MCP_HTTP_TOKEN"),
		MetricsAddr:  os.Getenv("MCP_METRICS_ADDR"),
		LogLevel:     envOr("LOG_LEVEL", "info"),
		LogFormat:    envOr("LOG_FORMAT", "text"),
	}

	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("NEWAPI_BASE_URL is required")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("NEWAPI_BASE_URL must be an absolute http(s) URL, got %q", cfg.BaseURL)
	}
	// A stray /api or trailing slash would double up in every request path.
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/api")

	if cfg.AccessToken == "" {
		return nil, fmt.Errorf("NEWAPI_ACCESS_TOKEN is required (mint one from New API: 个人设置 → 生成访问令牌)")
	}
	if cfg.Timeout < time.Second {
		return nil, fmt.Errorf("NEWAPI_TIMEOUT must be at least 1s, got %s", cfg.Timeout)
	}
	switch cfg.Transport {
	case TransportStdio:
	case TransportHTTP:
		if cfg.HTTPToken == "" {
			return nil, fmt.Errorf("MCP_HTTP_TOKEN is required for the http transport")
		}
	default:
		return nil, fmt.Errorf("MCP_TRANSPORT must be stdio or http, got %q", cfg.Transport)
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func durationOr(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func boolOr(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func floatOr(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return fallback
	}
	return f
}
