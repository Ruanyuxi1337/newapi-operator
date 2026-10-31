# Tool reference

All 47 tools, grouped by intent. Arguments marked `*` are required; everything
else is optional. Every tool returns JSON.

`R` tools only read. `W` tools can change state; clients see them with
`readOnlyHint: false`, and `NEWAPI_READ_ONLY=true` refuses them before a request
is built. In read-only mode `api_request` is refused for every method, including
`GET` — the curated read tools still work.

Quota arguments ending in `_usd` are converted with `NEWAPI_QUOTA_PER_UNIT`
(default `500000`, New API's "500,000 quota = $1"). Arguments ending in
`remain_quota` or `value` are raw internal units.

## Channels

| Tool | | Arguments | What it does |
|---|---|---|---|
| `channel_list` | R | `p`, `page_size`, `status`, `type`, `group`, `sort_by`, `sort_order`, `id_sort`, `tag_mode` | Paginated list with filters. `status` accepts `enabled`, `disabled`, `all`. |
| `channel_get` | R | `id*` | Full configuration of one channel (`base_url`, `models`, `model_mapping`, `setting`). |
| `channel_create` | W | `name*`, `type*`, `key*`, `base_url`, `models[]`, `model_mapping`, `group`, `priority`, `weight`, `auto_ban`, `mode`, `multi_key_mode`, `tag`, `extra` | Creates a channel and wraps the body in New API's `{mode, channel}` envelope. |
| `channel_update` | W | `id*`, plus any create field | Updates a channel. Sends `PUT /api/channel` with the id in the body. |
| `channel_delete` | W | `id*` | Permanently deletes a channel. Prefer `channel_set_status` to disable. |
| `channel_set_status` | W | `id*`, `status*` | Enables or disables one channel. `status` takes `enabled`, `disabled`, or the raw int (1 enabled, 2 manual disable, 3 auto disable). |
| `channel_batch_status` | W | `ids*[]`, `status*` | Same, for many channels in one call. |
| `channel_test` | R | `id*` | Runs New API's own probe through the channel and returns latency or the upstream error. |
| `channel_test_all` | R | — | Probes every channel. Slow on large instances. |
| `channel_fetch_models` | R | `id*` | Asks the upstream `/v1/models` what it actually serves. |
| `channel_balance` | W | `id` | Refreshes stored credit. Omit `id` to refresh every channel. |
| `channel_ops` | R | — | Aggregated channel health: failures, error counts, recent probes. |
| `channel_priority_order` | R | — | Reads the explicit channel selection order. |
| `channel_priority_reorder` | W | `ids*[]` | Replaces the order; the first id becomes highest priority. |
| `channel_codex_usage` | R | `id*` | Codex (type 57) channels: rate-limit windows and remaining credits. |
| `channel_codex_refresh` | W | `id*` | Forces an OAuth token refresh on a Codex channel. |
| `channel_upstream_detect` | R | `id` | Diffs the configured model list against what upstream publishes now. Omit `id` to scan all channels. |
| `channel_upstream_apply` | W | `id`, `add[]`, `remove[]` | Applies the changes found by `channel_upstream_detect`. |
| `channel_get_key` | R | `id*` | Returns the stored upstream key in full. Admin only — treat the output as a secret. |
| `channel_models` | R | `enabled_only` | Model ids known to the instance, optionally only those reachable through enabled channels. |
| `channel_copy` | W | `id*`, `name` | Clones a channel under a new name. |
| `channel_multi_key` | W | `id*`, `action*`, `key_index[]` | Manages the key pool inside a multi-key channel. `action`: `list`, `enable`, `disable`, `delete`; `key_index` is zero-based. |

## Downstream keys

| Tool | | Arguments | What it does |
|---|---|---|---|
| `token_list` | R | `p`, `page_size` | Lists keys with quota, model limits and group. Keys come back masked. |
| `token_get` | R | `id*` | One key, including its model allowlist and quota state. |
| `token_create` | W | `name*`, `models[]`, `remain_quota_usd`, `remain_quota`, `unlimited_quota`, `expired_time`, `group`, `allow_ips`, `allow_user_quota_query`, `extra` | Mints a key. `models` is the per-key allowlist — anything else is rejected before it reaches a channel. |
| `token_update` | W | `id*`, `name`, `models[]`, `model_limits_enabled`, `remain_quota`, `remain_quota_usd`, `unlimited_quota`, `expired_time`, `group`, `allow_ips`, `allow_user_quota_query`, `extra` | Changes a key. Sends `PUT /api/token` with the id in the body; `models` replaces the allowlist wholesale. |
| `token_delete` | W | `id*` | Revokes a key immediately. Irreversible. |
| `token_reveal_key` | R | `id*` | Returns the full plaintext key. |
| `token_batch_keys` | R | `ids*[]` | Returns several plaintext keys in one call. |
| `token_auto_groups` | R | — | Groups a token can be auto-placed in, with availability. |
| `token_search` | R | `keyword*`, `p`, `page_size` | Finds keys by name. |

## Users

| Tool | | Arguments | What it does |
|---|---|---|---|
| `user_list` | R | `p`, `page_size` | Lists accounts with role, status, used quota and group. |
| `user_get` | R | `id*` | One account by id. |
| `user_search` | R | `keyword`, `group`, `p`, `page_size` | Finds users by username, email or group. |
| `user_manage` | W | `id*`, `action*`, `amount_usd`, `value`, `mode` | One administrative action. `action`: `add_quota`, `enable`, `disable`, `promote`, `demote`, `delete`. `amount_usd` may be negative. |
| `user_access_tokens` | R | `include_catalog` | Fine-grained access tokens and their permission scopes. |
| `user_session_info` | R | — | New API's status document: version, feature flags, public settings. The cheapest connectivity check. |

## Settings, pricing and logs

| Tool | | Arguments | What it does |
|---|---|---|---|
| `option_list` | R | — | All system options: site name, registration policy, quota settings, rate limits, feature switches. |
| `option_set` | W | `key*`, `value*` | Writes one option. Values are sent as-is; several change billing or access immediately, so read first. |
| `pricing_get` | R | — | The per-model price table. Models absent from it fall back to a default ratio. |
| `pricing_patch` | W | `pricing*` | Patches pricing with the same shape `pricing_get` returned; only the keys you send change. |
| `pricing_preview` | R | `pricing*` | Computes what a pricing document would bill without applying it. |
| `log_list` | R | `p`, `page_size`, `username`, `token_name`, `model_name`, `channel`, `group`, `request_id`, `type`, `start_time`, `end_time` | Usage records. One row is one relayed request. Times are Unix seconds. |
| `log_stats` | R | `username`, `group`, `model_name`, `channel`, `start_time`, `end_time` | Aggregated quota, requests and token counts over a window. |

## Generic

| Tool | | Arguments | What it does |
|---|---|---|---|
| `api_request` | W | `method*`, `path*`, `query`, `body` | Calls any `/api/*` route. Path parameters use gin syntax (`/api/channel/:id`) but must be substituted before calling. |
| `api_endpoints` | R | `group`, `method`, `search`, `limit` | Searches the embedded route table. `limit` defaults to 200. |
| `api_coverage` | R | — | Reports the upstream release the table came from and the route count per group. |

## Recipes

**Create an OpenAI-compatible channel**

```json
{
  "name": "openai-main",
  "type": 1,
  "key": "sk-upstream",
  "base_url": "https://api.openai.com",
  "models": ["gpt-6-astra", "gpt-6-mini"],
  "group": "default"
}
```

`base_url` has no `/v1` suffix — New API appends `/v1/chat/completions` itself,
so a suffix produces `/v1/v1/…` and 404s.

**Issue a key restricted to two models and $5 of credit**

```json
{
  "name": "alice-flash-only",
  "models": ["gemini-2.5-flash", "qwen3.8-flash"],
  "remain_quota_usd": 5
}
```

**Find the channels that are failing**

Call `channel_ops` first; it returns New API's aggregated view. Then
`channel_test {"id": 7}` for the authoritative per-channel probe.

**Adopt a model the upstream added**

```
channel_upstream_detect {"id": 7}
→ proposed additions and removals
channel_upstream_apply {"id": 7, "add": ["gpt-6.1"]}
```

**Change one model's price**

Read with `pricing_get`, build the document, run `pricing_preview`, then
`pricing_patch`. Preview and patch accept the same shape.