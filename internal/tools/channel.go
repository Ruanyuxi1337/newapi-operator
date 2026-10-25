package tools

import (
	"context"
	"fmt"
	"strconv"
)

// channelTools returns the channel-family tool set.
func (r *Registry) channelTools() []Tool {
	return []Tool{
		r.channelList(),
		r.channelGet(),
		r.channelCreate(),
		r.channelUpdate(),
		r.channelDelete(),
		r.channelSetStatus(),
		r.channelBatchStatus(),
		r.channelTest(),
		r.channelTestAll(),
		r.channelFetchModels(),
		r.channelBalance(),
		r.channelOps(),
		r.channelPriorityOrder(),
		r.channelPriorityReorder(),
		r.channelCodexUsage(),
		r.channelCodexRefresh(),
		r.channelUpstreamDetect(),
		r.channelUpstreamApply(),
		r.channelGetKey(),
		r.channelModels(),
		r.channelCopy(),
		r.channelMultiKey(),
	}
}

func (r *Registry) channelList() Tool {
	return Tool{
		Name:        "channel_list",
		Title:       "List channels",
		Description: "List upstream channels with pagination and filters. Returns channel id, name, type, status, models, group, priority and balance.",
		Schema: object("Filters mirror New API's GET /api/channel/.",
			map[string]any{
				"p":          intProp("Page number, 1-based (default 1)."),
				"page_size":  intProp("Rows per page (default 100)."),
				"status":     enumProp("Filter by status: enabled, disabled, or all.", "enabled", "disabled", "all"),
				"type":       intProp("Filter by channel type id (1 = OpenAI, 3 = Azure, 57 = Codex, ...)."),
				"group":      strProp("Filter by channel group name."),
				"id_sort":    boolProp("Sort by id instead of name."),
				"tag_mode":   boolProp("Return tag rows instead of channels."),
				"sort_by":    strProp("Sort key understood by New API (e.g. id, name, priority)."),
				"sort_order": strProp("asc or desc."),
			}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			q := pageQuery(a)
			for _, k := range []string{"status", "group", "sort_by", "sort_order"} {
				if v := argString(a, k); v != "" {
					q.Set(k, v)
				}
			}
			if n, ok := argInt(a, "type"); ok {
				q.Set("type", strconv.Itoa(n))
			}
			for _, k := range []string{"id_sort", "tag_mode"} {
				if b, ok := argBool(a, k); ok {
					q.Set(k, strconv.FormatBool(b))
				}
			}
			return r.call(ctx, "GET", "/api/channel/", q, nil)
		},
	}
}

func (r *Registry) channelGet() Tool {
	return Tool{
		Name:        "channel_get",
		Title:       "Get one channel",
		Description: "Fetch a single channel by id, including its full configuration (base_url, models, model_mapping, setting).",
		Schema: object("", map[string]any{
			"id": intProp("Channel id."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "GET", fmt.Sprintf("/api/channel/%d", id), nil, nil)
		},
	}
}

// channelBody assembles the channel object New API expects from flat arguments.
func channelBody(a Args, includeID bool) (map[string]any, error) {
	body := map[string]any{}
	if includeID {
		id, err := requireInt(a, "id")
		if err != nil {
			return nil, err
		}
		body["id"] = id
	}
	if v := argString(a, "name"); v != "" {
		body["name"] = v
	}
	if n, ok := argInt(a, "type"); ok {
		body["type"] = n
	}
	if v := argString(a, "key"); v != "" {
		body["key"] = v
	}
	if v := argString(a, "base_url"); v != "" {
		body["base_url"] = v
	}
	if v, ok := a["models"]; ok {
		if s, ok := joinModels(v); ok {
			body["models"] = s
		}
	}
	if v := argString(a, "group"); v != "" {
		body["group"] = v
	}
	if n, ok := argInt(a, "priority"); ok {
		body["priority"] = n
	}
	if n, ok := argInt(a, "weight"); ok {
		body["weight"] = n
	}
	if v, ok := a["model_mapping"]; ok {
		if s, ok := jsonString(v); ok {
			body["model_mapping"] = s
		}
	}
	if v := argString(a, "tag"); v != "" {
		body["tag"] = v
	}
	if b, ok := argBool(a, "auto_ban"); ok {
		body["auto_ban"] = b
	}
	mergeExtra(body, a)
	return body, nil
}

func (r *Registry) channelCreate() Tool {
	channelSchema := map[string]any{
		"name":           strProp("Channel display name."),
		"type":           intProp("Channel type id: 1 = OpenAI-compatible, 3 = Azure, 14 = Anthropic, 24 = Gemini, 57 = Codex. See api_endpoints for the full type list source."),
		"key":            strProp("Upstream credential. For multi-key channels separate keys with a newline."),
		"base_url":       strProp("Upstream origin WITHOUT a trailing /v1 — New API appends /v1/chat/completions itself."),
		"models":         strArrayProp("Model ids this channel serves. Comma-separated string accepted too."),
		"group":          strProp("Channel group, e.g. default or vip."),
		"priority":       intProp("Higher wins when several channels serve the same model."),
		"weight":         intProp("Weighted round-robin share."),
		"model_mapping":  map[string]any{"type": "object", "description": "Client model name -> upstream model name. Stored JSON-encoded."},
		"tag":            strProp("Optional tag used for batch operations."),
		"auto_ban":       boolProp("Let New API disable the channel automatically on upstream failure."),
		"mode":           enumProp("single, or multi_to_single to split a newline-separated key list into separate credentials behind one channel.", "single", "multi_to_single"),
		"multi_key_mode": enumProp("Distribution across the multi-key pool.", "polling", "random"),
		"extra":          map[string]any{"type": "object", "description": "Raw channel fields merged last, for options not surfaced here (setting, header_override, param_override, ...)."},
	}
	return Tool{
		Name:        "channel_create",
		Title:       "Create a channel",
		Description: "Create an upstream channel. The body is wrapped in New API's {mode, channel} envelope automatically — a flat body is rejected by the API.",
		Mutating:    true,
		Schema:      object("", channelSchema, "name", "type", "key"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			ch, err := channelBody(a, false)
			if err != nil {
				return nil, err
			}
			// New API rejects unknown/empty required fields with a bare message,
			// so fail fast here with a specific one.
			for _, k := range []string{"name", "type", "key"} {
				if _, ok := ch[k]; !ok {
					return nil, fmt.Errorf("channel_create requires %q", k)
				}
			}
			mode := argString(a, "mode")
			if mode == "" {
				mode = "single"
			}
			wrapper := map[string]any{"mode": mode, "channel": ch}
			if mkm := argString(a, "multi_key_mode"); mkm != "" {
				wrapper["multi_key_mode"] = mkm
			}
			return r.call(ctx, "POST", "/api/channel/", nil, wrapper)
		},
	}
}

func (r *Registry) channelUpdate() Tool {
	return Tool{
		Name:        "channel_update",
		Title:       "Update a channel",
		Description: "Update a channel. Sends PUT /api/channel with the id inside the body — the /api/channel/:id form does not exist and returns 404.",
		Mutating:    true,
		Schema: object("Only the fields you pass are changed; New API merges the rest.", map[string]any{
			"id":            intProp("Channel id to update."),
			"name":          strProp("New display name."),
			"type":          intProp("New channel type id."),
			"key":           strProp("Replacement credential — use this to rotate a leaked key."),
			"base_url":      strProp("New upstream origin, without a trailing /v1."),
			"models":        strArrayProp("Replacement model list."),
			"group":         strProp("New group."),
			"priority":      intProp("New priority."),
			"weight":        intProp("New weight."),
			"model_mapping": map[string]any{"type": "object", "description": "Replacement model map."},
			"tag":           strProp("New tag."),
			"auto_ban":      boolProp("New auto-ban flag."),
			"extra":         map[string]any{"type": "object", "description": "Raw fields merged last."},
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			body, err := channelBody(a, true)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "PUT", "/api/channel/", nil, body)
		},
	}
}

func (r *Registry) channelDelete() Tool {
	return Tool{
		Name:        "channel_delete",
		Title:       "Delete a channel",
		Description: "Permanently delete a channel by id. Prefer channel_set_status to disable instead — deletion drops the credential and cannot be undone.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"id": intProp("Channel id to delete."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "DELETE", fmt.Sprintf("/api/channel/%d", id), nil, nil)
		},
	}
}

// channelStatusValue maps a friendly word onto New API's status codes.
// 1 = enabled, 2 = manually disabled, 3 = auto disabled.
func channelStatusValue(a Args) (int, error) {
	switch argString(a, "status") {
	case "enabled", "enable", "on":
		return 1, nil
	case "disabled", "disable", "off":
		return 2, nil
	}
	if n, ok := argInt(a, "status"); ok {
		return n, nil
	}
	return 0, fmt.Errorf(`status must be "enabled" or "disabled"`)
}

func (r *Registry) channelSetStatus() Tool {
	return Tool{
		Name:        "channel_set_status",
		Title:       "Enable or disable one channel",
		Description: "Toggle a single channel without touching its configuration. Use this when one account is rate-limited or dead.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"id":     intProp("Channel id."),
			"status": map[string]any{"description": `"enabled" / "disabled", or the raw New API status int (1 enabled, 2 manual disable, 3 auto disable).`},
		}, "id", "status"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			st, err := channelStatusValue(a)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "POST", fmt.Sprintf("/api/channel/%d/status", id), nil, map[string]any{"status": st})
		},
	}
}

func (r *Registry) channelBatchStatus() Tool {
	return Tool{
		Name:        "channel_batch_status",
		Title:       "Enable or disable many channels",
		Description: "Toggle a list of channels in one call — use it after a bulk key rotation or when a provider outage hits several accounts.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"ids":    intArrayProp("Channel ids to change."),
			"status": map[string]any{"description": `"enabled" / "disabled", or the raw status int.`},
		}, "ids", "status"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			raw, ok := a["ids"].([]any)
			if !ok || len(raw) == 0 {
				return nil, fmt.Errorf("ids must be a non-empty array of channel ids")
			}
			ids := make([]int, 0, len(raw))
			for _, v := range raw {
				f, ok := v.(float64)
				if !ok {
					return nil, fmt.Errorf("ids must contain only integers")
				}
				ids = append(ids, int(f))
			}
			st, err := channelStatusValue(a)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "POST", "/api/channel/status/batch", nil, map[string]any{"ids": ids, "status": st})
		},
	}
}

func (r *Registry) channelTest() Tool {
	return Tool{
		Name:        "channel_test",
		Title:       "Test one channel",
		Description: "Ask New API to send a probe request through one channel. Returns the upstream latency or the error — this is the authoritative health check.",
		Schema: object("", map[string]any{
			"id": intProp("Channel id."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "GET", fmt.Sprintf("/api/channel/test/%d", id), nil, nil)
		},
	}
}

func (r *Registry) channelTestAll() Tool {
	return Tool{
		Name:        "channel_test_all",
		Title:       "Test every channel",
		Description: "Run New API's health probe across all channels and return a per-channel verdict. Slow on large setups; prefer channel_test for a single check.",
		Schema:      object("", map[string]any{}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			return r.call(ctx, "GET", "/api/channel/test", nil, nil)
		},
	}
}

func (r *Registry) channelFetchModels() Tool {
	return Tool{
		Name:        "channel_fetch_models",
		Title:       "Fetch upstream model list",
		Description: "Ask New API to query the channel's upstream /v1/models and return what it actually serves. Use it to discover model ids before filling the channel's model list.",
		Schema: object("", map[string]any{
			"id": intProp("Channel id."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "GET", fmt.Sprintf("/api/channel/fetch_models/%d", id), nil, nil)
		},
	}
}

func (r *Registry) channelBalance() Tool {
	return Tool{
		Name:        "channel_balance",
		Title:       "Refresh upstream balance",
		Description: "Query the upstream provider for the channel's remaining credit and store it. Omit id to refresh every channel.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"id": intProp("Channel id. Omit to refresh all channels."),
		}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			if id, ok := argInt(a, "id"); ok {
				return r.call(ctx, "GET", fmt.Sprintf("/api/channel/update_balance/%d", id), nil, nil)
			}
			return r.call(ctx, "GET", "/api/channel/update_balance", nil, nil)
		},
	}
}

func (r *Registry) channelOps() Tool {
	return Tool{
		Name:        "channel_ops",
		Title:       "Channel operations overview",
		Description: "Return New API's aggregated channel health/operations view: which channels are failing, error counts and recent probe results.",
		Schema:      object("", map[string]any{}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			return r.call(ctx, "GET", "/api/channel/ops", nil, nil)
		},
	}
}

func (r *Registry) channelPriorityOrder() Tool {
	return Tool{
		Name:        "channel_priority_order",
		Title:       "Read channel priority order",
		Description: "Return the explicit priority ordering New API keeps for channel selection.",
		Schema:      object("", map[string]any{}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			return r.call(ctx, "GET", "/api/channel/priority/order", nil, nil)
		},
	}
}

func (r *Registry) channelPriorityReorder() Tool {
	return Tool{
		Name:        "channel_priority_reorder",
		Title:       "Rewrite channel priority order",
		Description: "Replace the channel priority ordering with the given id sequence. The list is applied in order: first id becomes highest priority.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"ids": intArrayProp("Channel ids, highest priority first."),
		}, "ids"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			raw, ok := a["ids"].([]any)
			if !ok || len(raw) == 0 {
				return nil, fmt.Errorf("ids must be a non-empty array of channel ids")
			}
			ids := make([]int, 0, len(raw))
			for _, v := range raw {
				f, ok := v.(float64)
				if !ok {
					return nil, fmt.Errorf("ids must contain only integers")
				}
				ids = append(ids, int(f))
			}
			return r.call(ctx, "POST", "/api/channel/priority/reorder", nil, map[string]any{"ids": ids})
		},
	}
}

func (r *Registry) channelCodexUsage() Tool {
	return Tool{
		Name:        "channel_codex_usage",
		Title:       "Codex channel usage",
		Description: "Read a Codex (ChatGPT OAuth) channel's rate-limit windows and remaining credits. Only meaningful for channels of type 57.",
		Schema: object("", map[string]any{
			"id": intProp("Channel id."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "GET", fmt.Sprintf("/api/channel/%d/codex/usage", id), nil, nil)
		},
	}
}

func (r *Registry) channelCodexRefresh() Tool {
	return Tool{
		Name:        "channel_codex_refresh",
		Title:       "Refresh Codex credential",
		Description: "Force a refresh of a Codex channel's OAuth token. Use it after the upstream starts answering 401 while the refresh token is still valid.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"id": intProp("Channel id."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "POST", fmt.Sprintf("/api/channel/%d/codex/refresh", id), nil, nil)
		},
	}
}

func (r *Registry) channelUpstreamDetect() Tool {
	return Tool{
		Name:        "channel_upstream_detect",
		Title:       "Detect upstream model changes",
		Description: "Compare the channel's configured model list against what the upstream publishes now. Omit id to scan every channel. Returns the proposed additions/removals without applying them.",
		Schema: object("", map[string]any{
			"id": intProp("Channel id. Omit to detect across all channels."),
		}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			if id, ok := argInt(a, "id"); ok {
				return r.call(ctx, "POST", "/api/channel/upstream_updates/detect", nil, map[string]any{"id": id})
			}
			return r.call(ctx, "POST", "/api/channel/upstream_updates/detect_all", nil, map[string]any{})
		},
	}
}

func (r *Registry) channelUpstreamApply() Tool {
	return Tool{
		Name:        "channel_upstream_apply",
		Title:       "Apply upstream model changes",
		Description: "Apply the changes found by channel_upstream_detect. Run detect first: this mutates the channel's model list.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"id":     intProp("Channel id. Omit to apply across all channels."),
			"add":    strArrayProp("Model ids to add. Defaults to what detect proposed when the server tracks it."),
			"remove": strArrayProp("Model ids to remove."),
		}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			body := map[string]any{}
			if id, ok := argInt(a, "id"); ok {
				body["id"] = id
			}
			if v, ok := a["add"]; ok {
				if list, ok := stringList(v); ok {
					body["add"] = list
				}
			}
			if v, ok := a["remove"]; ok {
				if list, ok := stringList(v); ok {
					body["remove"] = list
				}
			}
			if _, ok := argInt(a, "id"); ok {
				return r.call(ctx, "POST", "/api/channel/upstream_updates/apply", nil, body)
			}
			return r.call(ctx, "POST", "/api/channel/upstream_updates/apply_all", nil, body)
		},
	}
}

func (r *Registry) channelGetKey() Tool {
	return Tool{
		Name:        "channel_get_key",
		Title:       "Read a channel credential",
		Description: "Return a channel's stored upstream key in full. Root/admin only. Results are redacted by the caller's logging, not by New API.",
		Schema: object("", map[string]any{
			"id": intProp("Channel id."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "POST", fmt.Sprintf("/api/channel/%d/key", id), nil, map[string]any{})
		},
	}
}

func (r *Registry) channelModels() Tool {
	return Tool{
		Name:        "channel_models",
		Title:       "Models known to New API",
		Description: "List the model ids New API knows about, or only those reachable through enabled channels when enabled_only is true.",
		Schema: object("", map[string]any{
			"enabled_only": boolProp("Return only models served by at least one enabled channel."),
		}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			if b, ok := argBool(a, "enabled_only"); ok && b {
				return r.call(ctx, "GET", "/api/channel/models_enabled", nil, nil)
			}
			return r.call(ctx, "GET", "/api/channel/models", nil, nil)
		},
	}
}

func (r *Registry) channelCopy() Tool {
	return Tool{
		Name:        "channel_copy",
		Title:       "Duplicate a channel",
		Description: "Clone an existing channel under a new name — handy for adding a second key to the same upstream without retyping the config.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"id":   intProp("Source channel id."),
			"name": strProp("Name for the copy. Defaults to the source name plus a suffix."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			body := map[string]any{}
			if n := argString(a, "name"); n != "" {
				body["name"] = n
			}
			return r.call(ctx, "POST", fmt.Sprintf("/api/channel/copy/%d", id), nil, body)
		},
	}
}

func (r *Registry) channelMultiKey() Tool {
	return Tool{
		Name:        "channel_multi_key",
		Title:       "Manage a multi-key channel",
		Description: "Inspect or operate the key pool inside a multi-key channel (list, enable/disable, delete individual keys).",
		Mutating:    true,
		Schema: object("", map[string]any{
			"id":        intProp("Channel id."),
			"action":    enumProp("Operation to perform.", "list", "enable", "disable", "delete"),
			"key_index": intArrayProp("Zero-based indexes of the keys to affect."),
		}, "id", "action"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			body := map[string]any{"id": id, "action": argString(a, "action")}
			if v, ok := a["key_index"]; ok {
				body["key_index"] = v
			}
			return r.call(ctx, "POST", "/api/channel/multi_key/manage", nil, body)
		},
	}
}
