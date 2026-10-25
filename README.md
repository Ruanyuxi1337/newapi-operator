# newapi-operator

An MCP server that gives an AI agent **full administrative control** over a
self-hosted [New API](https://github.com/QuantumNous/new-api) LLM gateway —
channels, downstream keys, users, quotas, model pricing, usage logs, and every
other admin route the running release exposes.

Built in Go. Ships a route table extracted directly from New API's router source
at a pinned tag, plus a drift detector so the tool set is never silently out of
date.

```
┌─────────────┐   MCP (stdio/HTTP)   ┌──────────────────┐   /api/*   ┌─────────┐
│ AI agent    │ ◄──────────────────► │ newapi-operator  │ ◄────────► │ New API │
└─────────────┘                      └──────────────────┘            └─────────┘
```

---

## Why this exists

New API's admin API has a set of behaviours that quietly break naive automation:

| Behaviour | What happens if you assume otherwise |
|---|---|
| A failed call returns **HTTP 200** with `{"success": false}` | Every rejection looks like success |
| Channel create needs a **nested** `{"mode", "channel"}` body | Flat body is rejected |
| Channel/token update is `PUT /api/channel` with the **id in the body** | `PUT /api/channel/:id` returns 404 |
| `models` and `model_mapping` are **strings**, not arrays | Request is rejected |
| An OpenAI channel's `base_url` must **not** end in `/v1` | Requests go to `…/v1/v1/chat/completions` → 404 |
| Quota is stored in internal units (500 000 = $1) | Spend caps are off by 500 000× |

`newapi-operator` encodes each of those in a curated tool, and exposes the rest
of the surface through a generic pass-through so nothing is unreachable.

Every third-party MCP server for New API in the wild ships a **frozen** OpenAPI
snapshot. One popular one carries a snapshot two minor releases behind and
misses **55%** of the endpoints in `v1.0.0-rc.47`. This project ships the
generator instead: `cmd/routesync` rebuilds the table from upstream source and
fails CI when the table and the pinned release disagree.

---

## Install

Requires Go 1.24+.

```bash
git clone https://github.com/Ruanyuxi1337/newapi-operator.git
cd newapi-operator
make build          # bin/newapi-operator, bin/routesync
```

### Get an access token

The server authenticates with a New API **access token**, not a relay key:

> Log into your New API panel → avatar → **个人设置 / Personal settings** →
> **生成访问令牌 / Generate access token** → copy it.

Do not use your password: New API allows **one session per user**, so a script
that logs in with a password while your browser is open gets HTTP 409.

---

## Configure

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `NEWAPI_BASE_URL` | yes | — | Instance origin, e.g. `http://127.0.0.1:3000`. A trailing `/` or `/api` is stripped. |
| `NEWAPI_ACCESS_TOKEN` | yes | — | Admin access token. |
| `NEWAPI_USER_ID` | no | `1` | Sent as `New-Api-User`. Must be the token owner's numeric id. |
| `NEWAPI_TIMEOUT` | no | `60s` | Per-request timeout. |
| `NEWAPI_READ_ONLY` | no | `false` | Refuse every mutating tool. |
| `NEWAPI_QUOTA_PER_UNIT` | no | `500000` | Quota units per currency unit, used by `*_usd` arguments. |
| `MCP_TRANSPORT` | no | `stdio` | `stdio` or `http`. |
| `MCP_HTTP_ADDR` | no | `127.0.0.1:8080` | Listener for the `http` transport. |
| `MCP_HTTP_TOKEN` | http only | — | Bearer token required on `/mcp`. |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error`. |

**Call the container/gateway port directly, not a public domain.** Reverse
proxies that normalise trailing slashes answer 307 and drop the request body,
which silently breaks every create/update call.

---

## Wire it into Hermes Agent

```bash
hermes config set mcp_servers.newapi.command /abs/path/to/bin/newapi-operator
hermes config set mcp_servers.newapi.env.NEWAPI_BASE_URL http://127.0.0.1:3000
hermes config set mcp_servers.newapi.env.NEWAPI_ACCESS_TOKEN '<token>'
```

Restart Hermes. Tools appear as `mcp_newapi_*`. Hermes defers `mcp-*` toolsets
behind `tool_search`, so a large tool set costs no prompt budget until used.

## Wire it into any MCP client (JSON)

```json
{
  "mcpServers": {
    "newapi": {
      "command": "/abs/path/to/bin/newapi-operator",
      "env": {
        "NEWAPI_BASE_URL": "http://127.0.0.1:3000",
        "NEWAPI_ACCESS_TOKEN": "your-access-token",
        "NEWAPI_USER_ID": "1"
      }
    }
  }
}
```

### HTTP transport

```bash
NEWAPI_BASE_URL=http://127.0.0.1:3000 \
NEWAPI_ACCESS_TOKEN=… \
MCP_TRANSPORT=http MCP_HTTP_ADDR=127.0.0.1:8080 MCP_HTTP_TOKEN=$(openssl rand -hex 16) \
  ./bin/newapi-operator
```

`POST /mcp` with `Authorization: Bearer $MCP_HTTP_TOKEN`. `GET /healthz` is
unauthenticated and reports the binary version plus the route-table version.

---

## Tool surface

47 tools, split by intent. Mutating tools carry `readOnlyHint: false`, so a
client can gate them.

### Channels — `channel_*`

| Tool | Purpose |
|---|---|
| `channel_list` | Paginated list with status/type/group filters |
| `channel_get` | Full configuration of one channel |
| `channel_create` | Create (wraps the nested body for you) |
| `channel_update` | Update (id in body, `PUT /api/channel/`) |
| `channel_delete` | Delete — prefer `channel_set_status` |
| `channel_set_status` | Enable/disable one channel |
| `channel_batch_status` | Enable/disable many |
| `channel_test` / `channel_test_all` | New API's own upstream probe |
| `channel_fetch_models` | Ask the upstream what it actually serves |
| `channel_balance` | Refresh stored credit |
| `channel_ops` | Aggregated health view |
| `channel_priority_order` / `channel_priority_reorder` | Read/rewrite selection order |
| `channel_codex_usage` / `channel_codex_refresh` | Codex (type 57) rate-limit windows and token refresh |
| `channel_upstream_detect` / `channel_upstream_apply` | Diff and sync upstream model lists |
| `channel_get_key` | Reveal the stored upstream credential |
| `channel_models` | Models known to the instance |
| `channel_copy` | Duplicate a channel |
| `channel_multi_key` | Operate a multi-key pool |

### Downstream keys — `token_*`

| Tool | Purpose |
|---|---|
| `token_list` / `token_get` / `token_search` | Inspect keys |
| `token_create` | Mint a key; **`models` builds the per-key allowlist** |
| `token_update` | Change quota, allowlist, group, expiry |
| `token_delete` | Revoke |
| `token_reveal_key` / `token_batch_keys` | Full plaintext key(s) |
| `token_auto_groups` | Auto-group candidates |

### Users, settings, logs, generic

`user_list`, `user_get`, `user_search`, `user_manage` (add quota / enable /
disable / promote / demote / delete), `user_access_tokens`, `user_session_info`,
`option_list`, `option_set`, `pricing_get`, `pricing_patch`, `pricing_preview`,
`log_list`, `log_stats`, `api_endpoints`, `api_coverage`, `api_request`.

### The escape hatch

`api_request` calls **any** `/api/*` route: subscriptions, deployments, task
plugins, performance diagnostics, per-channel Ollama pulls — the whole 312-route
surface, not just what has a wrapper. Use `api_endpoints` to discover the exact
path and method:

```
api_endpoints {"group": "subscription"}
→ GET  /api/subscription/plans
  POST /api/subscription/admin/plans
  PATCH /api/subscription/admin/plans/:id
  …
```

---

## Per-key model permissions

The reason most people want a gateway in front of a gateway. `token_create`

```json
{
  "name": "alice-flash-only",
  "models": ["gemini-2.5-flash", "qwen3.8-flash"],
  "remain_quota_usd": 5
}
```

produces a key that can reach exactly those two models. A request for anything
else is rejected by New API **before** it reaches an upstream channel, so the
account pool behind the gateway never sees it.

Omit `models` for an unrestricted key. `remain_quota_usd` is converted to New
API's internal quota units using `NEWAPI_QUOTA_PER_UNIT`.

---

## Staying in sync with upstream

The route table lives in `internal/spec/routes_gen.go` and is generated, never
hand-edited.

```bash
make sync                       # regenerate from the latest New API release
./bin/routesync --tag v1.0.0-rc.47 --report   # per-group route counts
./bin/routesync --tag latest --check          # exit 1 if the table has drifted
```

`routesync` reads New API's `router/*.go` at the given tag and handles all three
registrations styles the project mixes:

```go
channelRoute.GET("/test/:id", handler)                        // direct
handlePermissionRoute(channelRoute, "GET", "/ops", perm, h)    // helper, often multi-line
var channelPermissionRoutes = []permissionRoute{               // table-driven — how the
    {method: http.MethodGet, path: "/priority/order", ...},    //   main CRUD is registered
}
```

A parser that only understands the first style misses the majority of the admin
surface; `internal/spec` covers all three and has tests for each.

**Automation:** `.github/workflows/upstream-drift.yml` runs weekly, resolves the
newest New API release, and opens (or updates) a GitHub issue with the exact
added/removed routes when the table falls behind.

To adopt a new upstream release:

```bash
make sync                                          # or: ./bin/routesync --tag vX.Y.Z
make sync-baseline                                 # refresh routes.baseline.json
git diff internal/spec                             # review what upstream added
go test ./...                                      # the drift test must pass again
```

---

## Safety model

- **Read-only mode** (`NEWAPI_READ_ONLY=true`) refuses every mutating tool at the
  registry layer, before a request is built.
- **Path allowlist** — the client refuses any path outside `/api/*`, so a mistake
  cannot turn this into an open proxy for relay traffic.
- **Credential redaction** — `internal/redact` scrubs bearer tokens, `sk-*` keys,
  JWTs and secret-shaped JSON fields from every error string before it reaches a
  log line or a tool result.
- **No ambient egress** — the only host this process talks to is
  `NEWAPI_BASE_URL`. There is no telemetry, no metrics listener unless you set
  one, and no `init()` side effects.
- **Delete is not disable** — deleting a channel or token is irreversible; the
  tool descriptions say so, and the agent instructions repeat it.

---

## Development

```bash
make build      # build both binaries
make test       # go test ./... -race
make vet        # go vet ./...
make fmt        # gofmt -w .
make sync       # regenerate the route table from latest upstream
make smoke      # end-to-end stdio test against a live instance
```

Layout:

```
cmd/newapi-operator/   MCP server entry point
cmd/routesync/         route-table generator and drift detector
internal/client/       admin HTTP client (envelope, error mapping, redaction)
internal/config/       environment configuration
internal/redact/       credential scrubbing
internal/spec/         route table, upstream parser, generated routes
internal/tools/        curated + generic tool set
scripts/smoke_test.py  stdio end-to-end check
```

## License

MIT — see [LICENSE](LICENSE).

Not affiliated with the New API project. It is an independent client for that
project's admin API.