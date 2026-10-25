package tools

import (
	"context"
	"fmt"
	"math"
	"net/url"
)

// userTools returns the account-management tool set.
func (r *Registry) userTools() []Tool {
	return []Tool{
		r.userList(),
		r.userGet(),
		r.userSearch(),
		r.userManage(),
		r.userAccessTokens(),
		r.userSessionInfo(),
	}
}

func (r *Registry) userList() Tool {
	return Tool{
		Name:        "user_list",
		Title:       "List users",
		Description: "List the instance's user accounts with role, status, used quota and group.",
		Schema: object("", map[string]any{
			"p":         intProp("Page number."),
			"page_size": intProp("Rows per page."),
		}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			return r.call(ctx, "GET", "/api/user/", pageQuery(a), nil)
		},
	}
}

func (r *Registry) userGet() Tool {
	return Tool{
		Name:        "user_get",
		Title:       "Get one user",
		Description: "Fetch a single user account by id.",
		Schema: object("", map[string]any{
			"id": intProp("User id."),
		}, "id"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			return r.call(ctx, "GET", fmt.Sprintf("/api/user/%d", id), nil, nil)
		},
	}
}

func (r *Registry) userSearch() Tool {
	return Tool{
		Name:        "user_search",
		Title:       "Search users",
		Description: "Find users by username, email or group.",
		Schema: object("", map[string]any{
			"keyword":   strProp("Match against username / display name / email."),
			"group":     strProp("Group filter."),
			"p":         intProp("Page number."),
			"page_size": intProp("Rows per page."),
		}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			q := pageQuery(a)
			for _, k := range []string{"keyword", "group"} {
				if v := argString(a, k); v != "" {
					q.Set(k, v)
				}
			}
			return r.call(ctx, "GET", "/api/user/search", q, nil)
		},
	}
}

func (r *Registry) userManage() Tool {
	return Tool{
		Name:        "user_manage",
		Title:       "Manage a user account",
		Description: "Apply one administrative action to a user: add quota, enable, disable, promote, demote or delete. Billing-affecting — check the id first with user_get.",
		Mutating:    true,
		Schema: object("", map[string]any{
			"id":         intProp("User id."),
			"action":     enumProp("Operation to perform.", "add_quota", "enable", "disable", "promote", "demote", "delete"),
			"amount_usd": numProp("For add_quota: how much to add, in currency units ($). Negative values subtract."),
			"value":      numProp("For add_quota: raw quota units, if you prefer to bypass the conversion."),
			"mode":       strProp("For add_quota: New API's balance mode flag, when the instance uses one."),
		}, "id", "action"),
		Handler: func(ctx context.Context, a Args) (any, error) {
			id, err := requireInt(a, "id")
			if err != nil {
				return nil, err
			}
			action := argString(a, "action")
			if action == "" {
				return nil, fmt.Errorf("action is required")
			}
			body := map[string]any{"id": id, "action": action}
			if v, ok := a["amount_usd"]; ok {
				f, ok := v.(float64)
				if !ok {
					return nil, fmt.Errorf("amount_usd must be a number")
				}
				body["value"] = int64(math.Round(f * r.quotaPerUnit))
			} else if n, ok := argInt(a, "value"); ok {
				body["value"] = n
			}
			if m := argString(a, "mode"); m != "" {
				body["mode"] = m
			}
			return r.call(ctx, "POST", "/api/user/manage", nil, body)
		},
	}
}

func (r *Registry) userAccessTokens() Tool {
	return Tool{
		Name:        "user_access_tokens",
		Title:       "Inspect scoped access tokens",
		Description: "List the fine-grained access tokens (and the permission scopes each can carry) that New API exposes for automation. Useful when the token this server uses needs to be re-scoped.",
		Schema: object("", map[string]any{
			"include_catalog": boolProp("Also return the permission catalogue (every scope the instance recognises)."),
		}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			out, err := r.call(ctx, "GET", "/api/user/access_tokens/", nil, nil)
			if err != nil {
				return nil, err
			}
			if b, ok := argBool(a, "include_catalog"); ok && b {
				cat, catErr := r.call(ctx, "GET", "/api/user/access_tokens/catalog", nil, nil)
				if catErr == nil {
					return map[string]any{"tokens": out, "catalog": cat}, nil
				}
			}
			return out, nil
		},
	}
}

func (r *Registry) userSessionInfo() Tool {
	return Tool{
		Name:        "user_session_info",
		Title:       "Instance and session snapshot",
		Description: "Return New API's own status document — version, enabled feature flags and public settings. The cheapest way to confirm what the running instance supports before calling anything else.",
		Schema:      object("", map[string]any{}),
		Handler: func(ctx context.Context, a Args) (any, error) {
			return r.call(ctx, "GET", "/api/status", url.Values{}, nil)
		},
	}
}
