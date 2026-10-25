package tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ruanyuxi1337/newapi-operator/internal/client"
)

// capture records the last request the mock New API received.
type capture struct {
	method string
	path   string
	query  string
	body   map[string]any
}

func newTestRegistry(t *testing.T, resp string, readOnly bool) (*Registry, *capture) {
	t.Helper()
	cap := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.method = r.Method
		cap.path = r.URL.Path
		cap.query = r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &cap.body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)

	c := client.NewWithOptions(client.Options{
		BaseURL:  srv.URL,
		Token:    "t",
		UserID:   "1",
		HTTP:     &http.Client{Timeout: 5 * time.Second},
		ReadOnly: readOnly,
	})
	return NewRegistry(c, 500000), cap
}

func findTool(t *testing.T, r *Registry, name string) Tool {
	t.Helper()
	for _, tool := range r.Tools() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not registered", name)
	return Tool{}
}

// channel_create must nest the body; a flat channel object is rejected by the API.
func TestChannelCreateNestsBody(t *testing.T) {
	r, cap := newTestRegistry(t, `{"success":true,"data":1}`, false)
	r.Add(r.channelTools()...)

	tool := findTool(t, r, "channel_create")
	_, err := tool.Handler(context.Background(), Args{
		"name":     "qwen-maas",
		"type":     float64(1),
		"key":      "sk-upstream",
		"base_url": "https://maas.example.com/compatible-mode",
		"models":   []any{"deepseek-v4.1-flash-1017", "qwen3.8-max"},
		"priority": float64(10),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cap.method != "POST" || cap.path != "/api/channel/" {
		t.Fatalf("request = %s %s", cap.method, cap.path)
	}
	if cap.body["mode"] != "single" {
		t.Errorf("mode = %v, want single", cap.body["mode"])
	}
	nested, ok := cap.body["channel"].(map[string]any)
	if !ok {
		t.Fatalf("body is not nested under \"channel\": %+v", cap.body)
	}
	if nested["models"] != "deepseek-v4.1-flash-1017,qwen3.8-max" {
		t.Errorf("models must be a comma-separated string, got %#v", nested["models"])
	}
	if nested["base_url"] != "https://maas.example.com/compatible-mode" {
		t.Errorf("base_url = %v", nested["base_url"])
	}
}

// channel_update must PUT /api/channel with the id in the body; the /:id form 404s.
func TestChannelUpdateCarriesIDInBody(t *testing.T) {
	r, cap := newTestRegistry(t, `{"success":true}`, false)
	r.Add(r.channelTools()...)

	if _, err := findTool(t, r, "channel_update").Handler(context.Background(), Args{
		"id":   float64(42),
		"name": "renamed",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.method != "PUT" || cap.path != "/api/channel/" {
		t.Fatalf("request = %s %s, want PUT /api/channel/", cap.method, cap.path)
	}
	if cap.body["id"] != float64(42) {
		t.Errorf("id missing from body: %+v", cap.body)
	}
}

// token_create must convert a currency amount into quota units and turn the model
// allowlist into the two fields New API actually bills against.
func TestTokenCreateConvertsQuotaAndModels(t *testing.T) {
	r, cap := newTestRegistry(t, `{"success":true,"data":{"id":5}}`, false)
	r.Add(r.tokenTools()...)

	if _, err := findTool(t, r, "token_create").Handler(context.Background(), Args{
		"name":             "alice-flash-only",
		"models":           []any{"gemini-2.5-flash"},
		"remain_quota_usd": float64(5),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.method != "POST" || cap.path != "/api/token/" {
		t.Fatalf("request = %s %s", cap.method, cap.path)
	}
	if got := cap.body["remain_quota"]; got != float64(5*500000) {
		t.Errorf("remain_quota = %v, want %v", got, float64(5*500000))
	}
	if cap.body["model_limits"] != "gemini-2.5-flash" {
		t.Errorf("model_limits = %#v", cap.body["model_limits"])
	}
	if cap.body["model_limits_enabled"] != true {
		t.Errorf("model_limits_enabled must be true when an allowlist is sent, got %#v", cap.body["model_limits_enabled"])
	}
}

func TestTokenCreateWithoutModelsLeavesAllowlistOff(t *testing.T) {
	r, cap := newTestRegistry(t, `{"success":true}`, false)
	r.Add(r.tokenTools()...)

	if _, err := findTool(t, r, "token_create").Handler(context.Background(), Args{
		"name": "everything",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cap.body["model_limits_enabled"]; ok {
		t.Errorf("model_limits_enabled must not be forced when no allowlist was given: %+v", cap.body)
	}
}

// The registry — not the individual tool — enforces read-only mode.
func TestRegistryBlocksMutatingToolsInReadOnly(t *testing.T) {
	r, cap := newTestRegistry(t, `{"success":true}`, true)
	r.Add(r.channelTools()...)

	tool := findTool(t, r, "channel_delete")

	// The handler itself is pure; the guard lives in the adapter, so exercise it
	// the way the MCP layer does.
	if !tool.Mutating {
		t.Fatal("channel_delete must be marked mutating")
	}
	if _, err := r.client.Do(context.Background(), "DELETE", "/api/channel/1", nil, nil); !errors.Is(err, client.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
	if cap.method != "" {
		t.Errorf("upstream was reached in read-only mode: %s %s", cap.method, cap.path)
	}
}

func TestAPIRequestRejectsNonAdminPath(t *testing.T) {
	r, _ := newTestRegistry(t, `{"success":true}`, false)
	r.Add(r.genericTools()...)

	if _, err := findTool(t, r, "api_request").Handler(context.Background(), Args{
		"method": "GET",
		"path":   "/v1/models",
	}); err == nil {
		t.Fatal("api_request must refuse paths outside /api/")
	}
}

func TestAPIRequestPassesQueryAndBody(t *testing.T) {
	r, cap := newTestRegistry(t, `{"success":true,"data":[]}`, false)
	r.Add(r.genericTools()...)

	if _, err := findTool(t, r, "api_request").Handler(context.Background(), Args{
		"method": "POST",
		"path":   "/api/subscription/admin/plans",
		"query":  map[string]any{"p": float64(2), "all": true},
		"body":   map[string]any{"title": "pro"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.path != "/api/subscription/admin/plans" {
		t.Errorf("path = %q", cap.path)
	}
	if cap.query != "all=true&p=2" {
		t.Errorf("query = %q", cap.query)
	}
	if cap.body["title"] != "pro" {
		t.Errorf("body = %+v", cap.body)
	}
}

// Every registered tool must carry a usable schema, or the MCP client rejects it.
func TestAllToolsHaveValidSchema(t *testing.T) {
	r, _ := newTestRegistry(t, `{"success":true}`, false)
	RegisterAll(nil, r) // tool lists are populated before Attach

	for _, tool := range r.Tools() {
		if tool.Name == "" {
			t.Error("tool with empty name")
		}
		if tool.Description == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
		if tool.Schema == nil {
			t.Errorf("tool %q has no schema", tool.Name)
			continue
		}
		if tool.Schema["type"] != "object" {
			t.Errorf("tool %q schema type = %v, want object", tool.Name, tool.Schema["type"])
		}
		if _, ok := tool.Schema["properties"]; !ok {
			t.Errorf("tool %q schema has no properties", tool.Name)
		}
	}
}

func TestToolNamesAreUnique(t *testing.T) {
	r, _ := newTestRegistry(t, `{"success":true}`, false)
	RegisterAll(nil, r)

	seen := map[string]bool{}
	for _, tool := range r.Tools() {
		if seen[tool.Name] {
			t.Fatalf("duplicate tool name %q", tool.Name)
		}
		seen[tool.Name] = true
	}
}
