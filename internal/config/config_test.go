package config

import (
	"strings"
	"testing"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestLoadRequiresBaseURLAndToken(t *testing.T) {
	setEnv(t, map[string]string{"NEWAPI_BASE_URL": "", "NEWAPI_ACCESS_TOKEN": ""})
	if _, err := Load(); err == nil {
		t.Fatal("expected an error when NEWAPI_BASE_URL is missing")
	}

	setEnv(t, map[string]string{"NEWAPI_BASE_URL": "http://localhost:3000", "NEWAPI_ACCESS_TOKEN": ""})
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "NEWAPI_ACCESS_TOKEN") {
		t.Fatalf("expected a missing-token error, got %v", err)
	}
}

// A stray /api or trailing slash would double up in every request path.
func TestLoadTrimsBaseURLSuffixes(t *testing.T) {
	cases := map[string]string{
		"http://localhost:3000/":     "http://localhost:3000",
		"http://localhost:3000/api":  "http://localhost:3000",
		"http://localhost:3000/api/": "http://localhost:3000",
	}
	for in, want := range cases {
		setEnv(t, map[string]string{
			"NEWAPI_BASE_URL": in, "NEWAPI_ACCESS_TOKEN": "t",
		})
		cfg, err := Load()
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if cfg.BaseURL != want {
			t.Errorf("%s -> %q, want %q", in, cfg.BaseURL, want)
		}
	}
}

func TestLoadRejectsNonHTTPURL(t *testing.T) {
	setEnv(t, map[string]string{"NEWAPI_BASE_URL": "file:///etc/passwd", "NEWAPI_ACCESS_TOKEN": "t"})
	if _, err := Load(); err == nil {
		t.Fatal("expected a rejection of a non-http scheme")
	}
}

func TestLoadHTTPTransportNeedsToken(t *testing.T) {
	setEnv(t, map[string]string{
		"NEWAPI_BASE_URL":     "http://localhost:3000",
		"NEWAPI_ACCESS_TOKEN": "t",
		"MCP_TRANSPORT":       "http",
		"MCP_HTTP_TOKEN":      "",
	})
	if _, err := Load(); err == nil {
		t.Fatal("http transport without MCP_HTTP_TOKEN must be rejected")
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, map[string]string{
		"NEWAPI_BASE_URL":     "http://localhost:3000",
		"NEWAPI_ACCESS_TOKEN": "t",
		"NEWAPI_USER_ID":      "",
		"NEWAPI_READ_ONLY":    "true",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UserID != "1" {
		t.Errorf("UserID = %q, want 1", cfg.UserID)
	}
	if !cfg.ReadOnly {
		t.Error("ReadOnly should be true")
	}
	if cfg.QuotaPerUnit != 500000 {
		t.Errorf("QuotaPerUnit = %v, want 500000", cfg.QuotaPerUnit)
	}
	if cfg.Transport != TransportStdio {
		t.Errorf("Transport = %q, want stdio", cfg.Transport)
	}
}
