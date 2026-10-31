# Architecture

`newapi-operator` is a thin, opinionated layer between an MCP client and one New
API instance. It has three jobs:

1. Translate tool arguments into the request shapes New API actually accepts.
2. Turn New API's ambiguous responses into unambiguous tool results.
3. Keep the addressable route surface in lockstep with upstream releases.

It is deliberately not a general-purpose HTTP proxy, a cache, or a daemon.

```
MCP client
   │  tools/call
   ▼
internal/tools    tool definitions, JSON schemas, handlers
   │  Registry.adapt: decode args, read-only guard, JSON result
   ▼
internal/client   path allowlist, auth headers, envelope decode, error mapping
   │  HTTP
   ▼
New API  /api/*
```

## Packages

| Path | Responsibility |
|---|---|
| `cmd/newapi-operator` | Process entry point: config, transports, signal handling. |
| `cmd/routesync` | Build-time and CI tool: extract routes from an upstream tag, diff, write the table and baseline. |
| `internal/config` | Environment loading and validation. Fails fast. |
| `internal/client` | One HTTP client for `/api/*`: headers, envelope parsing, sentinel errors, redaction of error strings. |
| `internal/tools` | The 47 tools, declared as data (`Tool{Name, Description, Schema, Mutating, Handler}`), plus the generic pass-through. |
| `internal/spec` | The generated route table, the upstream Go-source parser, and `Resolve` for trailing-slash normalisation. |
| `internal/redact` | Regex scrubbing of credential-shaped strings. |

## Request lifecycle

1. **Adapt.** `Registry.adapt` decodes the MCP arguments into `Args`. A mutating
   tool is refused here when `NEWAPI_READ_ONLY=true`, before any HTTP work.
2. **Build.** The handler maps tool arguments onto New API's body. This is where
   the non-obvious contracts live: `channel_create` wraps `{mode, channel}`,
   `channel_update` puts the id in the body, `models` becomes a comma-separated
   string, `*_usd` becomes quota units.
3. **Send.** `client.Do` rejects anything outside `/api/*`, sets
   `Authorization: Bearer …` and `New-Api-User`, and reads at most 16 MiB.
4. **Classify.** The body is parsed as New API's envelope. `success: false`
   becomes an `APIError` even though the HTTP status is 200. Non-2xx responses
   map to sentinel errors (`ErrNotFound`, `ErrUnauth`, `ErrRateLimit`,
   `ErrUpstream`, `ErrBadRequest`) that callers and tests can match with
   `errors.Is`; transport errors are returned as redacted prose.
5. **Return.** The handler returns a value; the adapter encodes it as an
   indented JSON text result. Errors become `isError: true` results rather than
   protocol errors, so the agent sees the message and can react.

## The route table

`internal/spec/routes_gen.go` is generated from a pinned New API tag. It is the
source of truth for `api_endpoints` and for trailing-slash resolution in
`api_request`.

New API registers routes in three different styles, and a real release uses all
three:

```go
// 1. Direct registration.
channelRoute.GET("/test/:id", handler)

// 2. Permission helper, frequently wrapped across lines.
handlePermissionRoute(channelRoute, "GET", "/ops", PermissionRoot, h)

// 3. Table-driven registration — most CRUD routes.
var channelPermissionRoutes = []permissionRoute{
    {method: http.MethodGet, path: "/priority/order", permission: PermissionRoot, handler: h},
}
```

`internal/spec/parse.go` handles all three. `parse_test.go` has a case per style;
a regex that only matches style 1 silently drops most of the admin surface, so
the tests exist to make that failure loud.

`routes.baseline.json` is the committed snapshot CI compares against.
`routesync --check` regenerates from the requested tag and exits non-zero on any
difference. The weekly drift workflow resolves the newest release, runs that
check, and files an issue containing the exact added and removed routes.

A tag is used rather than `main` on purpose: the embedded table should describe a
release someone can run, not an unreleased tree.

## Transports

- **stdio** (default) — the client spawns the process. All logs go to stderr;
  stdout belongs to the protocol.
- **HTTP** — `mcp.NewStreamableHTTPHandler` behind a constant-time bearer check
  on `/mcp`. `/healthz` is public and exposes only the binary version and route
  table version.

There is no third mode and no implicit listener.

## Security boundaries

| Boundary | Enforcement |
|---|---|
| What can be called | `client.Do` requires a path beginning with `/api/`. |
| What can be written | `NEWAPI_READ_ONLY=true` blocks mutating tools at the registry, and non-GET methods again at the client. |
| What can leak | Error strings pass through `internal/redact`; log records never include the token. Successful reveal tools (`channel_get_key`, `token_reveal_key`, `token_batch_keys`) return plaintext by design — the boundary there is who can reach the MCP server, so expose it to trusted clients only. |
| Where traffic goes | Only `NEWAPI_BASE_URL` (plus Go's standard HTTP proxy environment, if one is set). No telemetry. |

## Failure modes worth knowing

- **Reverse proxies.** A proxy that normalises a trailing slash answers 307 and
  can drop the body on `POST`/`PUT`. The client resolves known paths against the
  route table to avoid the mismatch, but the reliable fix is to call the gateway
  port directly.
- **One session per user.** Password login is not supported by design; an access
  token avoids the 409 that a second session triggers.
- **Deleted means deleted.** `channel_delete` and `token_delete` are not soft
  deletes upstream. Tool descriptions and server instructions both say to prefer
  `channel_set_status` / `token_update` when the goal is to stop usage.

## Adding a tool

1. Add a `Tool` in the matching `internal/tools/*.go` file and register it in the
   file's `*Tools()` slice.
2. Set `Mutating: true` for anything that is not a GET.
3. Add a handler test in `internal/tools/tools_test.go` using `httptest`, and
   assert the exact method, path and body New API receives.
4. If the route is new in a newer New API release, run `make sync` first so
   `Resolve` knows about it.
5. Update `docs/TOOLS.md`.