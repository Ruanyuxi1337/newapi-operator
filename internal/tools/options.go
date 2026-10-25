package tools

import (
	"context"
	"net/url"
	"strconv"
)

// optionTools returns instance-settings tools, including model pricing.
//
// Pricing matters more than it looks: New API bills any model missing from its
// price table at a default ratio, so a per-usage token handed out before the
// table is populated produces numbers that do not match the operator's intent.
func (r *Registry) optionTools() []Tool {
	return []Tool{
		r.optionList(),
		r.optionSet(),
		r.pricingGet(),
		r.pricingPatch(),
		r.pricingPreview(),
	}
}

func (r *Registry) optionList() Tool {
	return Tool{
		Name:        "option_list",
		Title:       "Read instance settings",
		Description: "Return the instance's system options (site name, registration policy, quota settings, rate limits and feature switches).",
		Schema:      object("", map[string]any{}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			return r.call(ctx, "GET", "/api/option/", nil, nil)
		},
	}
}

func (r *Registry) optionSet() Tool {
	return Tool{
		Name:        "option_set",
		Title:       "Change a setting",
		Description: "Write one system option. This is a broad switch — confirm the current value with option_list first, because several options change billing or access behaviour immediately.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"key":   strProp("Option key exactly as returned by option_list."),
			"value": map[string]any{"description": "New value. Strings, numbers and booleans are accepted; it is sent as-is."},
		}, "key", "value"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			key := argString(a, "key")
			if key == "" {
				return nil, errString("key is required")
			}
			v, ok := a["value"]
			if !ok {
				return nil, errString("value is required")
			}
			return r.call(ctx, "PUT", "/api/option/", nil, map[string]any{"key": key, "value": valueString(v)})
		},
	}
}

// valueString renders an option value the way New API's PUT /api/option expects:
// a string in the JSON body, regardless of the underlying type.
func valueString(v any) any {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return v
	}
}

func (r *Registry) pricingGet() Tool {
	return Tool{
		Name:        "pricing_get",
		Title:       "Read the model price table",
		Description: "Return the per-model pricing configuration New API bills against. Models absent here fall back to a default ratio, which is why per-usage tokens can look mischarged.",
		Schema:      object("", map[string]any{}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			return r.call(ctx, "GET", "/api/option/model_pricing", nil, nil)
		},
	}
}

func (r *Registry) pricingPatch() Tool {
	return Tool{
		Name:        "pricing_patch",
		Title:       "Update the model price table",
		Description: "Patch model pricing. Send the pricing document in the same shape pricing_get returned (PATCH semantics: only the keys you send change).",
		Mutating:    true,
		Schema: object("", map[string]any{
			"pricing": map[string]any{"type": "object", "description": "Pricing document. Use pricing_get to learn the exact shape for this instance, then send only the models you are changing."},
		}, "pricing"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			body, ok := a["pricing"]
			if !ok {
				return nil, errString("pricing is required")
			}
			return r.call(ctx, "PATCH", "/api/option/model_pricing", nil, body)
		},
	}
}

func (r *Registry) pricingPreview() Tool {
	return Tool{
		Name:        "pricing_preview",
		Title:       "Preview pricing changes",
		Description: "Ask New API to compute what a pricing document would bill, without applying it. Run this before pricing_patch.",
		Schema: object("", map[string]any{
			"pricing": map[string]any{"type": "object", "description": "Candidate pricing document."},
		}, "pricing"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			body, ok := a["pricing"]
			if !ok {
				return nil, errString("pricing is required")
			}
			return r.call(ctx, "POST", "/api/option/model_pricing/preview", nil, body)
		},
	}
}

// logTools returns the usage-log tools.
func (r *Registry) logTools() []Tool {
	return []Tool{
		{
			Name:        "log_list",
			Title:       "Read usage logs",
			Description: "List usage records with filters. Each row is one relayed request: user, token, channel, model, tokens consumed and quota charged.",
			Schema: object("", map[string]any{
				"p":          intProp("Page number."),
				"page_size":  intProp("Rows per page."),
				"type":       intProp("Log type filter as defined by New API (0 = all)."),
				"username":   strProp("Filter by username."),
				"token_name": strProp("Filter by token label."),
				"model_name": strProp("Filter by model id."),
				"channel":    intProp("Filter by channel id."),
				"start_time": intProp("Unix seconds, inclusive."),
				"end_time":   intProp("Unix seconds, inclusive."),
				"group":      strProp("Filter by group."),
				"request_id": strProp("Filter by upstream request id."),
			}),
			Handler: func(ctx context.Context, a Args) (any, error) {
				q := pageQuery(a)
				for _, k := range []string{"username", "token_name", "model_name", "group", "request_id"} {
					if v := argString(a, k); v != "" {
						q.Set(k, v)
					}
				}
				for _, k := range []string{"type", "channel", "start_time", "end_time"} {
					if n, ok := argInt(a, k); ok {
						q.Set(k, strconv.Itoa(n))
					}
				}
				return r.call(ctx, "GET", "/api/log/", q, nil)
			},
		},
		{
			Name:        "log_stats",
			Title:       "Usage statistics",
			Description: "Aggregated usage over a time window, grouped by the instance's default dimension (quota consumed, requests and token counts).",
			Schema: object("", map[string]any{
				"start_time": intProp("Unix seconds, inclusive."),
				"end_time":   intProp("Unix seconds, inclusive."),
				"username":   strProp("Restrict to one user."),
				"model_name": strProp("Restrict to one model."),
				"channel":    intProp("Restrict to one channel id."),
				"group":      strProp("Restrict to one group."),
			}),
			Handler: func(ctx context.Context, a Args) (any, error) {
				q := url.Values{}
				for _, k := range []string{"username", "model_name", "group"} {
					if v := argString(a, k); v != "" {
						q.Set(k, v)
					}
				}
				for _, k := range []string{"start_time", "end_time", "channel"} {
					if n, ok := argInt(a, k); ok {
						q.Set(k, strconv.Itoa(n))
					}
				}
				return r.call(ctx, "GET", "/api/log/stat", q, nil)
			},
		},
	}
}
