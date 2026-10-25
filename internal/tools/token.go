package tools

import (
	"context"
	"fmt"
	"math"
)

// tokenTools returns the downstream-key tool set.
//
// A "token" in New API is the key handed to other people. It carries the quota
// and — the reason most operators care — the per-key model allowlist.
func (r *Registry) tokenTools() []Tool {
	return []Tool{
		r.tokenList(),
		r.tokenGet(),
		r.tokenCreate(),
		r.tokenUpdate(),
		r.tokenDelete(),
		r.tokenRevealKey(),
		r.tokenBatchKeys(),
		r.tokenAutoGroups(),
		r.tokenSearch(),
	}
}

func (r *Registry) tokenList() Tool {
	return Tool{
		Name:        "token_list",
		Title:       "List downstream keys",
		Description: "List tokens (the keys you hand to other people) with quota, used quota, model limits and group. Keys come back masked.",
		Schema: object("", map[string]any{
			"p":         intProp("Page number, 1-based."),
			"page_size": intProp("Rows per page (default 100)."),
		}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			return r.call(ctx, "GET", "/api/token/", pageQuery(a), nil)
		},
	}
}

func (r *Registry) tokenGet() Tool {
	return Tool{
		Name:        "token_get",
		Title:       "Get one key",
		Description: "Fetch a single token by id, including its model allowlist and quota state.",
		Schema: object("", map[string]any{
			"id": intProp("Token id."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "GET", fmt.Sprintf("/api/token/%d", id), nil, nil)
		},
	}
}

// tokenBody assembles a token payload from flat arguments.
//
// remain_quota is expressed in raw New API quota units, which are meaningless to
// a human, so remain_quota_usd is accepted and converted.
func (r *Registry) tokenBody(a Args) (map[string]any, error) {
	body := map[string]any{}
	if v := argString(a, "name"); v != "" {
		body["name"] = v
	}
	if b, ok := argBool(a, "unlimited_quota"); ok {
		body["unlimited_quota"] = b
	}
	if v, ok := a["remain_quota_usd"]; ok {
		f, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("remain_quota_usd must be a number")
		}
		body["remain_quota"] = int64(math.Round(f * r.quotaPerUnit))
	} else if n, ok := a["remain_quota"]; ok {
		f, ok := n.(float64)
		if !ok {
			return nil, fmt.Errorf("remain_quota must be a number")
		}
		body["remain_quota"] = int64(f)
	}
	if v, ok := a["models"]; ok {
		list, ok := stringList(v)
		if !ok {
			return nil, fmt.Errorf("models must be an array of strings or a comma-separated string")
		}
		// Setting the allowlist only takes effect when the flag is on.
		body["model_limits_enabled"] = len(list) > 0
		body["model_limits"] = joinList(list)
	}
	if b, ok := argBool(a, "model_limits_enabled"); ok {
		body["model_limits_enabled"] = b
	}
	if v := argString(a, "allow_ips"); v != "" {
		body["allow_ips"] = v
	}
	if v := argString(a, "group"); v != "" {
		body["group"] = v
	}
	if n, ok := argInt(a, "expired_time"); ok {
		body["expired_time"] = n
	}
	if b, ok := argBool(a, "allow_user_quota_query"); ok {
		body["allow_user_quota_query"] = b
	}
	mergeExtra(body, a)
	return body, nil
}

func joinList(list []string) string {
	out := ""
	for i, s := range list {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

func (r *Registry) tokenCreate() Tool {
	return Tool{
		Name:        "token_create",
		Title:       "Create a downstream key",
		Description: "Mint a token to hand to someone else. Pass `models` to build the per-key allowlist — a request for any other model is rejected by New API before it reaches an upstream channel. Use remain_quota_usd for a spend cap (omit both quota fields for unlimited).",
		Mutating:    true,
		Schema: object("", map[string]any{
			"name":                   strProp("Token label, max 50 chars. Use something identifying like 'alice-flash-only'."),
			"models":                 strArrayProp("Model allowlist. Empty/omitted means every model the instance serves."),
			"remain_quota_usd":       numProp("Spend cap in currency units ($). Converted to New API quota units with the configured NEWAPI_QUOTA_PER_UNIT."),
			"remain_quota":           numProp("Spend cap in raw New API quota units, if you prefer to bypass the conversion."),
			"unlimited_quota":        boolProp("Ignore the quota entirely."),
			"expired_time":           intProp("Unix expiry timestamp. -1 means never."),
			"group":                  strProp("Token group; only channels in the same group serve it."),
			"allow_ips":              strProp("Comma-separated IP allowlist. Empty allows any source."),
			"allow_user_quota_query": boolProp("Let the holder query their own remaining quota."),
			"extra":                  map[string]any{"type": "object", "description": "Raw token fields merged last."},
		}, "name"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			body, err := r.tokenBody(a)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "POST", "/api/token/", nil, body)
		},
	}
}

func (r *Registry) tokenUpdate() Tool {
	return Tool{
		Name:        "token_update",
		Title:       "Update a downstream key",
		Description: "Change an existing token. Sends PUT /api/token with the id in the body — the /api/token/:id form does not exist. Passing `models` replaces the allowlist wholesale.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"id":                     intProp("Token id."),
			"name":                   strProp("New label."),
			"models":                 strArrayProp("Replacement model allowlist. Pass an empty array to clear it."),
			"model_limits_enabled":   boolProp("Force the allowlist on or off without changing its contents."),
			"remain_quota_usd":       numProp("New spend cap in $."),
			"remain_quota":           numProp("New spend cap in raw quota units."),
			"unlimited_quota":        boolProp("Toggle unlimited quota."),
			"expired_time":           intProp("New expiry (unix seconds), -1 for never."),
			"group":                  strProp("New group."),
			"allow_ips":              strProp("New IP allowlist."),
			"allow_user_quota_query": boolProp("Toggle self-service quota queries."),
			"extra":                  map[string]any{"type": "object", "description": "Raw token fields merged last."},
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			body, err := r.tokenBody(a)
			if err != nil {
				return nil, err
			}
			body["id"] = id
			return r.call(ctx, "PUT", "/api/token/", nil, body)
		},
	}
}

func (r *Registry) tokenDelete() Tool {
	return Tool{
		Name:        "token_delete",
		Title:       "Delete a downstream key",
		Description: "Permanently revoke a token. The holder's key stops working immediately.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"id": intProp("Token id."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "DELETE", fmt.Sprintf("/api/token/%d", id), nil, nil)
		},
	}
}

func (r *Registry) tokenRevealKey() Tool {
	return Tool{
		Name:        "token_reveal_key",
		Title:       "Reveal a downstream key",
		Description: "Return the full plaintext key for a token. List endpoints only show a mask, so this is how you actually hand the credential to its owner.",
		Schema: object("", map[string]any{
			"id": intProp("Token id."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "POST", fmt.Sprintf("/api/token/%d/key", id), nil, map[string]any{})
		},
	}
}

func (r *Registry) tokenBatchKeys() Tool {
	return Tool{
		Name:        "token_batch_keys",
		Title:       "Reveal many keys at once",
		Description: "Return full plaintext keys for several tokens in one call.",
		Schema: object("", map[string]any{
			"ids": intArrayProp("Token ids."),
		}, "ids"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			raw, ok := a["ids"].([]any)
			if !ok || len(raw) == 0 {
				return nil, fmt.Errorf("ids must be a non-empty array of token ids")
			}
			ids := make([]int, 0, len(raw))
			for _, v := range raw {
				f, ok := v.(float64)
				if !ok {
					return nil, fmt.Errorf("ids must contain only integers")
				}
				ids = append(ids, int(f))
			}
			return r.call(ctx, "POST", "/api/token/batch/keys", nil, map[string]any{"ids": ids})
		},
	}
}

func (r *Registry) tokenAutoGroups() Tool {
	return Tool{
		Name:        "token_auto_groups",
		Title:       "Auto-group candidates",
		Description: "List the groups a token may be automatically placed in, with their availability.",
		Schema:      object("", map[string]any{}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			return r.call(ctx, "GET", "/api/token/auto-groups", nil, nil)
		},
	}
}

func (r *Registry) tokenSearch() Tool {
	return Tool{
		Name:        "token_search",
		Title:       "Search keys by name",
		Description: "Find tokens whose name matches a keyword.",
		Schema: object("", map[string]any{
			"keyword":   strProp("Name fragment to match."),
			"p":         intProp("Page number."),
			"page_size": intProp("Rows per page."),
		}, "keyword"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			q := pageQuery(a)
			q.Set("keyword", argString(a, "keyword"))
			return r.call(ctx, "GET", "/api/token/search", q, nil)
		},
	}
}
