# Security policy

## Supported versions

Security fixes land on the latest release. Older releases are not patched; the
upgrade path is a static binary or a rebuild.

| Version | Supported |
|---|---|
| 0.2.x | yes |
| < 0.2 | no |

## Reporting a vulnerability

Use GitHub's private vulnerability reporting:
<https://github.com/Ruanyuxi1337/newapi-operator/security/advisories/new>

Please do not open a public issue for anything that could be exploited before a
fix ships. Include the version (`newapi-operator --version`), the configuration
that matters (transport, read-only flag — never the token itself), and a
reproduction.

Reports about New API itself belong at
<https://github.com/QuantumNous/new-api/security>.

## What counts

- A path that escapes the `/api/*` allowlist.
- A way to reach a mutating route while `NEWAPI_READ_ONLY=true`.
- A credential that survives `internal/redact` in an error string or a log line.
- An authentication bypass on the HTTP transport.
- Traffic to a host other than `NEWAPI_BASE_URL` that the operator did not
  configure.

## Deploying safely

This process holds an admin access token and can delete channels, keys and
users. Treat it as an admin control plane, not a public API.

- Run it on the same host or private network as New API. The recommended HTTP
  listener is `127.0.0.1:8080`.
- If you expose the HTTP transport beyond localhost, terminate TLS in front of
  it and set a long random `MCP_HTTP_TOKEN`.
- Use `NEWAPI_READ_ONLY=true` for agents that only observe.
- `channel_get_key`, `token_reveal_key` and `token_batch_keys` return plaintext
  credentials by design. Limit which MCP clients can call the server.
- Revoke the New API access token when a client no longer needs it.