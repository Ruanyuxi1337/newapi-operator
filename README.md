# newapi-operator

[![CI](https://github.com/Ruanyuxi1337/newapi-operator/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Ruanyuxi1337/newapi-operator/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Ruanyuxi1337/newapi-operator.svg)](https://pkg.go.dev/github.com/Ruanyuxi1337/newapi-operator)
[![Latest release](https://img.shields.io/github/v/release/Ruanyuxi1337/newapi-operator)](https://github.com/Ruanyuxi1337/newapi-operator/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

English | [中文](README.zh-CN.md)

An MCP server that gives an AI agent full administrative control over a
self-hosted [New API](https://github.com/QuantumNous/new-api) LLM gateway:
channels, downstream keys, users, quota, pricing and usage logs — plus a
pass-through to every other admin route the running release exposes.

```
AI agent  ◄── MCP (stdio / HTTP) ──►  newapi-operator  ◄── /api/* ──►  New API
```

47 curated tools · 312-route pass-through · route table generated from
upstream source, with a weekly drift check.

## Quickstart

Requires Go 1.26 or newer.

```bash
git clone https://github.com/Ruanyuxi1337/newapi-operator.git
cd newapi-operator
make build
```

Or install just the server binary with Go (Go 1.26+):

```bash
go install github.com/Ruanyuxi1337/newapi-operator/cmd/newapi-operator@latest
```

Create an access token in New API: **avatar → Personal settings → Generate
access token**.

> Use an access token, not a relay key (`sk-…`) and not your password. New API
> allows one session per user, so a password login from a script returns 409
> while the panel is open in a browser.

Register the binary with any MCP client that speaks stdio:

```json
{
  "mcpServers": {
    "newapi": {
      "command": "/absolute/path/to/bin/newapi-operator",
      "env": {
        "NEWAPI_BASE_URL": "http://127.0.0.1:3001",
        "NEWAPI_ACCESS_TOKEN": "nap_…",
        "NEWAPI_USER_ID": "1"
      }
    }
  }
}
```

Restart the client and ask the agent for `user_session_info`. It returns the
running New API version and feature flags — the fastest way to confirm the base
URL and token are right.

## What the wrapper fixes

New API's admin API is not plain REST. These are the contracts a naive client
gets wrong, and the reason the curated tools exist:

| Contract | A naive client |
|---|---|
| Rejections return **HTTP 200** with `{"success": false}` | Treats every error as success |
| Channel creation expects a nested `{"mode": …, "channel": {…}}` body | Sends a flat body and is rejected |
| Updates use `PUT /api/channel` with the **id in the body** | Calls `PUT /api/channel/:id` and gets 404 |
| `models` and `model_mapping` are **strings** | Sends arrays and is rejected |
| OpenAI channel `base_url` must **not** end in `/v1` | Requests hit `/v1/v1/chat/completions` and 404 |
| Quota uses internal units (**500,000 = $1**) | Spend caps are off by 500,000× |

The server turns `success: false` into a tool error, so a returned payload always
means the call actually worked.

## Configuration

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `NEWAPI_BASE_URL` | yes | — | Instance origin, e.g. `http://127.0.0.1:3001`. Trailing `/` and `/api` are stripped. |
| `NEWAPI_ACCESS_TOKEN` | yes | — | Admin access token. |
| `NEWAPI_USER_ID` | no | `1` | Sent as `New-Api-User`; must be the token owner's numeric id. |
| `NEWAPI_TIMEOUT` | no | `60s` | Per-request timeout. |
| `NEWAPI_READ_ONLY` | no | `false` | Refuse every mutating tool. |
| `NEWAPI_QUOTA_PER_UNIT` | no | `500000` | Quota units per currency unit, used by `*_usd` arguments. |
| `MCP_TRANSPORT` | no | `stdio` | `stdio` or `http`. |
| `MCP_HTTP_ADDR` | no | `127.0.0.1:8080` | Listener for the HTTP transport. |
| `MCP_HTTP_TOKEN` | http only | — | Bearer token required on `/mcp`. |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error`. |
| `LOG_FORMAT` | no | `text` | `text` or `json`. |

Startup fails fast when a required value is missing or the base URL, timeout or
transport is invalid.

Call the gateway port directly, not a public domain. Reverse proxies that
normalise trailing slashes answer 307 and drop the request body, which breaks
every create and update call.

## Tools

| Area | Count | Tools |
|---|---|---|
| Channels | 22 | `channel_list`, `channel_create`, `channel_test`, `channel_balance`, `channel_priority_reorder`, `channel_upstream_apply`, `channel_multi_key`, … |
| Downstream keys | 9 | `token_list`, `token_create`, `token_update`, `token_reveal_key`, `token_batch_keys`, … |
| Users | 6 | `user_list`, `user_manage`, `user_session_info`, `user_access_tokens`, … |
| Settings, pricing, logs | 7 | `option_list`, `option_set`, `pricing_get`, `pricing_patch`, `log_list`, `log_stats`, … |
| Generic | 3 | `api_request`, `api_endpoints`, `api_coverage` |

Full argument reference: [docs/TOOLS.md](docs/TOOLS.md).

### The escape hatch

`api_request` calls any `/api/*` route, including the parts with no dedicated
tool: subscriptions, deployments, task plugins, performance diagnostics, Ollama
pulls. Discover the path first:

```
api_endpoints {"group": "subscription"}
→ GET   /api/subscription/plans
  POST  /api/subscription/admin/plans
  PATCH /api/subscription/admin/plans/:id
```

Substitute path parameters before calling, e.g. `/api/channel/:id` →
`/api/channel/7`.

## Staying current with New API

The route table lives in `internal/spec/routes_gen.go` and is generated — never
hand-edited.

```bash
make sync                              # regenerate from the newest release
./bin/routesync --tag latest --report  # per-group route counts
./bin/routesync --tag latest --check   # exit 1 if the table has drifted
```

`routesync` reads `router/*.go` at the target tag and handles all three
registration styles New API mixes:

```go
channelRoute.GET("/test/:id", handler)                       // direct
handlePermissionRoute(channelRoute, "GET", "/ops", perm, h)   // helper, often wrapped
var channelPermissionRoutes = []permissionRoute{              // table-driven — most CRUD
    {method: http.MethodGet, path: "/priority/order", ...},
}
```

A parser that only understands the first style misses most of the admin surface;
`internal/spec` covers all three, with tests for each.

To adopt a new release:

```bash
make sync && make sync-baseline
git diff internal/spec     # see exactly what upstream added or removed
go test ./...
```

A weekly [workflow](.github/workflows/upstream-drift.yml) opens an issue when the
committed table falls behind the latest release.

## Deployment

### stdio

The default. Your MCP client starts the binary; all logging goes to stderr so it
never corrupts the protocol stream.

### HTTP

```bash
NEWAPI_BASE_URL=http://127.0.0.1:3001 \
NEWAPI_ACCESS_TOKEN=… \
MCP_TRANSPORT=http MCP_HTTP_ADDR=127.0.0.1:8080 \
MCP_HTTP_TOKEN=$(openssl rand -hex 16) \
  ./bin/newapi-operator
```

`POST /mcp` requires `Authorization: Bearer $MCP_HTTP_TOKEN`. `GET /healthz` is
unauthenticated and reports the binary version and route-table version.

### Docker

```bash
docker build -t newapi-operator .

docker run --rm --network host \
  -e NEWAPI_BASE_URL=http://127.0.0.1:3001 \
  -e NEWAPI_ACCESS_TOKEN=… \
  -e MCP_TRANSPORT=http \
  -e MCP_HTTP_ADDR=0.0.0.0:8080 \
  -e MCP_HTTP_TOKEN=$(openssl rand -hex 16) \
  newapi-operator
```

`--network host` is Linux-only. Elsewhere, point `NEWAPI_BASE_URL` at a host the
container can reach and publish port 8080.

## Security model

- **Read-only mode** (`NEWAPI_READ_ONLY=true`) refuses mutating tools at the
  registry layer, before a request is built.
- **Path allowlist** — the client refuses paths outside `/api/*`, so the server
  cannot become an open proxy for relay traffic.
- **Credential redaction** — `internal/redact` scrubs bearer tokens, `sk-*` keys,
  JWTs and secret-shaped JSON fields before they reach a log line or an error
  returned to the agent.
- **No ambient egress** — the only host contacted is `NEWAPI_BASE_URL`. No
  telemetry, no listener unless you set `MCP_TRANSPORT=http`.
- **Delete is not disable** — deleting a channel or token is irreversible; tool
  descriptions say so, and the agent instructions repeat it.

See [SECURITY.md](SECURITY.md) for reporting a vulnerability.

## Development

```bash
make build      # build both binaries
make test       # go test ./... -race
make vet        # go vet ./...
make fmt        # gofmt
make sync       # regenerate the route table from the latest release
make smoke      # stdio end-to-end check against a live instance
```

```
cmd/newapi-operator/   MCP server
cmd/routesync/         route-table generator and drift detector
internal/client/       admin HTTP client (envelope, errors, redaction)
internal/config/       environment configuration
internal/redact/       credential scrubbing
internal/spec/         route table, upstream parser, generated routes
internal/tools/        curated and generic tools
docs/                  tool reference and design notes
```

See [CONTRIBUTING.md](CONTRIBUTING.md) before opening a PR and
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how the pieces fit together.

## License

MIT — see [LICENSE](LICENSE). Not affiliated with the New API project; this is an
independent client for its admin API.