package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// errString is a tiny helper so handlers can return a one-line error inline.
func errString(s string) error { return fmt.Errorf("%s", s) }

// RegisterAll builds the full tool set and attaches it to the MCP server.
func RegisterAll(s *mcp.Server, r *Registry) {
	r.Add(r.channelTools()...)
	r.Add(r.tokenTools()...)
	r.Add(r.userTools()...)
	r.Add(r.optionTools()...)
	r.Add(r.logTools()...)
	r.Add(r.genericTools()...)
	// A nil server is allowed so tests can build the tool set without a transport.
	if s != nil {
		r.Attach(s)
	}
}

// Instructions is the server-level guidance handed to the agent on connect.
func Instructions(version, upstream string) string {
	return fmt.Sprintf(`New API operator — full administrative control over a self-hosted New API LLM gateway (route table extracted from New API %s).

Start here:
- user_session_info — confirm the instance answers and which version/features it runs.
- api_endpoints — discover the exact path for anything not covered by a curated tool; api_request can then call it.
- api_coverage — see how many routes the embedded table knows and from which release.

Working rules:
- Curated tools encode New API's non-obvious contracts. Prefer them: channel_create nests the body, channel_update and token_update carry the id IN the body, base_url must not end in /v1, model lists are comma-separated strings.
- A tool that returns data succeeded. New API reports rejections as HTTP 200 with success:false; this server converts those into errors, so do not re-check a payload for success flags.
- Quota is stored in internal units (default 500000 per currency unit). Curated tools accept *_usd and convert; api_request does not.
- Deleting a channel or token is irreversible. Prefer disabling when the goal is "stop it being used".
- Before handing a key to another person, set its model allowlist (token_create models) so it cannot reach models you did not intend to share.

Server %s.`, upstream, version)
}
