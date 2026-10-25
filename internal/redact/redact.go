// Package redact scrubs credential-shaped substrings out of anything that can
// reach a log line, an MCP tool result, or an error string.
//
// The rule is deliberately blunt: New API never needs to echo a credential back
// to us, so anything that looks like one is replaced before it leaves the
// process boundary.
package redact

import (
	"regexp"
	"strings"
)

var patterns = []*regexp.Regexp{
	// Bearer tokens.
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`),
	// OpenAI / New API style keys.
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`),
	// Anthropic keys.
	regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{8,}`),
	// JWTs (New API id_token / access_token are JWTs).
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
	// JSON fields whose name says "this is a secret".
	regexp.MustCompile(`(?i)"(access_token|refresh_token|id_token|api_key|key|password|secret|token)"\s*:\s*"[^"]*"`),
	// key=value forms in query strings and logs.
	regexp.MustCompile(`(?i)\b(access_token|refresh_token|api_key|apikey|key|password|secret|token)=[^&\s"']+`),
}

const mask = "[REDACTED]"

// String returns s with credential-shaped substrings masked.
func String(s string) string {
	for _, re := range patterns {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			switch {
			case strings.HasPrefix(strings.ToLower(m), "bearer"):
				// keep the scheme, drop the value
				if i := strings.IndexAny(m, " \t"); i >= 0 {
					return m[:i+1] + mask
				}
				return m
			case strings.HasPrefix(m, `"`):
				// keep the JSON field name, blank the value
				if i := strings.Index(m, ":"); i >= 0 {
					return m[:i+1] + `"` + mask + `"`
				}
				return mask
			case strings.Contains(m, "="):
				if i := strings.Index(m, "="); i >= 0 {
					return m[:i+1] + mask
				}
				return mask
			default:
				return mask
			}
		})
	}
	return s
}

// Bytes is String for byte slices.
func Bytes(b []byte) string { return String(string(b)) }
