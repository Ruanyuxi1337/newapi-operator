package tools

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Ruanyuxi1337/newapi-operator/internal/spec"
)

// genericTools exposes the whole admin surface even where no curated tool exists.
//
// The curated tools cover the operations that need judgement (nested bodies,
// body-carried ids, unit conversion). Everything else — 300+ routes across
// subscriptions, deployments, plugins, task runners — is still reachable here,
// so the agent is never blocked by a missing wrapper.
func (r *Registry) genericTools() []Tool {
	return []Tool{
		r.apiRequest(),
		r.apiEndpoints(),
		r.apiCoverage(),
	}
}

func (r *Registry) apiRequest() Tool {
	return Tool{
		Name:  "api_request",
		Title: "Call any New API admin endpoint",
		Description: `Raw pass-through for the New API admin API (/api/*). Use it when no curated tool exists — subscriptions, deployments, task plugins, performance diagnostics, per-channel codex usage, etc.

Path parameters use gin syntax as shown by api_endpoints, e.g. /api/channel/:id — substitute the real value before calling (/api/channel/7).

New API answers a rejected call with HTTP 200 and {"success": false}; this tool returns that as a structured error, so a returned payload means the call really succeeded.`,
		Mutating: true,
		Schema: object("", map[string]any{
			"method": enumProp("HTTP method.", "GET", "POST", "PUT", "PATCH", "DELETE"),
			"path":   strProp("Absolute admin path beginning with /api/. Example: /api/subscription/admin/plans"),
			"query":  map[string]any{"type": "object", "description": "Query string parameters. Values may be strings, numbers or booleans.", "additionalProperties": true},
			"body":   map[string]any{"description": "JSON request body. Objects and arrays are sent as-is; strings that already contain JSON are sent verbatim.", "type": []any{"object", "array", "string", "null"}},
		}, "method", "path"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			method := strings.ToUpper(argString(a, "method"))
			path := argString(a, "path")
			if path == "" {
				return nil, errString("path is required")
			}
			if !strings.HasPrefix(path, "/api/") && path != "/api" {
				return nil, errString("path must begin with /api/")
			}
			q, err := toQuery(a["query"])
			if err != nil {
				return nil, err
			}
			var body any
			if v, ok := a["body"]; ok {
				body = v
			}
			return r.call(ctx, method, path, q, body)
		},
	}
}

// toQuery converts the decoded JSON object into url.Values.
func toQuery(v any) (url.Values, error) {
	q := url.Values{}
	if v == nil {
		return q, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errString("query must be an object")
	}
	for k, raw := range m {
		switch t := raw.(type) {
		case string:
			q.Set(k, t)
		case float64:
			if t == float64(int64(t)) {
				q.Set(k, strconv.FormatInt(int64(t), 10))
			} else {
				q.Set(k, strconv.FormatFloat(t, 'f', -1, 64))
			}
		case bool:
			q.Set(k, strconv.FormatBool(t))
		case []any:
			for _, e := range t {
				q.Add(k, fmt.Sprint(e))
			}
		default:
			return nil, fmt.Errorf("unsupported query value for %q", k)
		}
	}
	return q, nil
}

func (r *Registry) apiEndpoints() Tool {
	return Tool{
		Name:        "api_endpoints",
		Title:       "Discover admin endpoints",
		Description: fmt.Sprintf("Search the embedded New API route table (%d admin routes extracted from %s). Filter by group, method or substring to find the path to pass to api_request.", len(spec.Routes), spec.UpstreamVersion),
		Schema: object("", map[string]any{
			"group":  strProp("Restrict to one route group, e.g. channel, token, user, option, subscription, plugin."),
			"method": strProp("Restrict to one HTTP method."),
			"search": strProp("Case-insensitive substring match on the path."),
			"limit":  intProp("Maximum rows to return (default 200)."),
		}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			group := strings.ToLower(argString(a, "group"))
			method := strings.ToUpper(argString(a, "method"))
			search := strings.ToLower(argString(a, "search"))
			limit := 200
			if n, ok := argInt(a, "limit"); ok && n > 0 {
				limit = n
			}

			rows := make([]spec.Route, 0, len(spec.Routes))
			for _, rt := range spec.Routes {
				if group != "" && strings.ToLower(rt.Group) != group {
					continue
				}
				if method != "" && rt.Method != method {
					continue
				}
				if search != "" && !strings.Contains(strings.ToLower(rt.Path), search) {
					continue
				}
				rows = append(rows, rt)
			}
			total := len(rows)
			if len(rows) > limit {
				rows = rows[:limit]
			}
			return map[string]any{
				"upstream_version": spec.UpstreamVersion,
				"total_matches":    total,
				"returned":         len(rows),
				"routes":           rows,
			}, nil
		},
	}
}

func (r *Registry) apiCoverage() Tool {
	return Tool{
		Name:        "api_coverage",
		Title:       "Route table coverage",
		Description: "Report which New API release the embedded route table came from and how many routes each group contributes. Compare it with the version the instance reports to spot an out-of-date table.",
		Schema:      object("", map[string]any{}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			counts := map[string]int{}
			for _, rt := range spec.Routes {
				counts[rt.Group]++
			}
			groups := make([]string, 0, len(counts))
			for g := range counts {
				groups = append(groups, g)
			}
			sort.Slice(groups, func(i, j int) bool {
				if counts[groups[i]] != counts[groups[j]] {
					return counts[groups[i]] > counts[groups[j]]
				}
				return groups[i] < groups[j]
			})
			return map[string]any{
				"upstream_version": spec.UpstreamVersion,
				"total_routes":     len(spec.Routes),
				"curated_tools":    len(r.tools),
				"groups":           counts,
				"groups_by_volume": groups,
			}, nil
		},
	}
}
