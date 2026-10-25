package tools

import (
	"context"
	"testing"

	"github.com/Ruanyuxi1337/newapi-operator/internal/spec"
)

// Every curated tool must call a path that actually exists in the release the
// table was generated from. This is the regression test for the trailing-slash
// and renamed-endpoint class of bug: the tool still compiles and the mock still
// answers, but against a real gateway it would 404 or — worse — 301 a POST into
// a bodyless GET.
func TestCuratedToolsHitRegisteredPaths(t *testing.T) {
	cases := []struct {
		tool string
		args Args
	}{
		{"channel_list", Args{}},
		{"channel_get", Args{"id": float64(1)}},
		{"channel_create", Args{"name": "x", "type": float64(1), "key": "k"}},
		{"channel_update", Args{"id": float64(1), "name": "x"}},
		{"channel_delete", Args{"id": float64(1)}},
		{"channel_set_status", Args{"id": float64(1), "status": "enabled"}},
		{"channel_batch_status", Args{"ids": []any{float64(1)}, "status": "disabled"}},
		{"channel_test", Args{"id": float64(1)}},
		{"channel_test_all", Args{}},
		{"channel_fetch_models", Args{"id": float64(1)}},
		{"channel_balance", Args{"id": float64(1)}},
		{"channel_ops", Args{}},
		{"channel_priority_order", Args{}},
		{"channel_priority_reorder", Args{"ids": []any{float64(1)}}},
		{"channel_codex_usage", Args{"id": float64(1)}},
		{"channel_codex_refresh", Args{"id": float64(1)}},
		{"channel_upstream_detect", Args{"id": float64(1)}},
		{"channel_upstream_apply", Args{"id": float64(1)}},
		{"channel_get_key", Args{"id": float64(1)}},
		{"channel_models", Args{"enabled_only": true}},
		{"channel_copy", Args{"id": float64(1)}},
		{"channel_multi_key", Args{"id": float64(1), "action": "list"}},

		{"token_list", Args{}},
		{"token_get", Args{"id": float64(1)}},
		{"token_search", Args{"keyword": "x"}},
		{"token_create", Args{"name": "x"}},
		{"token_update", Args{"id": float64(1)}},
		{"token_delete", Args{"id": float64(1)}},
		{"token_reveal_key", Args{"id": float64(1)}},
		{"token_batch_keys", Args{"ids": []any{float64(1)}}},
		{"token_auto_groups", Args{}},

		{"user_list", Args{}},
		{"user_get", Args{"id": float64(1)}},
		{"user_search", Args{"keyword": "x"}},
		{"user_manage", Args{"id": float64(1), "action": "enable"}},
		{"user_access_tokens", Args{}},
		{"user_session_info", Args{}},

		{"option_list", Args{}},
		{"option_set", Args{"key": "SiteName", "value": "x"}},
		{"pricing_get", Args{}},
		{"pricing_patch", Args{"pricing": map[string]any{}}},
		{"pricing_preview", Args{"pricing": map[string]any{}}},

		{"log_list", Args{}},
		{"log_stats", Args{}},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			r, cap := newTestRegistry(t, `{"success":true,"data":{}}`, false)
			RegisterAll(nil, r)

			tool := findTool(t, r, tc.tool)
			if _, err := tool.Handler(context.Background(), tc.args); err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if cap.path == "" {
				t.Fatal("handler made no request")
			}
			if _, ok := spec.Match(cap.method, cap.path); !ok {
				t.Errorf("%s called %s %s, which matches no route in the %s table",
					tc.tool, cap.method, cap.path, spec.UpstreamVersion)
			}
		})
	}
}

// Generic pass-through accepts anything under /api/, including paths this build
// has never heard of — that is the whole point of the escape hatch.
func TestAPIRequestAllowsUnknownButValidPaths(t *testing.T) {
	r, cap := newTestRegistry(t, `{"success":true}`, false)
	RegisterAll(nil, r)

	if _, err := findTool(t, r, "api_request").Handler(context.Background(), Args{
		"method": "GET",
		"path":   "/api/some-future-endpoint",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.path != "/api/some-future-endpoint" {
		t.Errorf("path = %q", cap.path)
	}
}

// api_request should still normalise a known path's trailing slash, so a caller
// who copies the path from docs cannot turn a POST into a redirect.
func TestAPIRequestNormalisesKnownPath(t *testing.T) {
	r, cap := newTestRegistry(t, `{"success":true}`, false)
	RegisterAll(nil, r)

	if _, err := findTool(t, r, "api_request").Handler(context.Background(), Args{
		"method": "POST",
		"path":   "/api/channel",
		"body":   map[string]any{"mode": "single"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.path != "/api/channel/" {
		t.Errorf("path = %q, want the registered spelling /api/channel/", cap.path)
	}
}
