package redact

import (
	"strings"
	"testing"
)

// Nothing that looks like a credential may survive redaction, in any of the
// shapes New API and its upstreams emit.
func TestStringMasksCredentials(t *testing.T) {
	cases := []struct {
		name string
		in   string
		leak string
	}{
		{"bearer", "request failed: Authorization: Bearer abcdef1234567890", "abcdef1234567890"},
		{"openai key", "upstream said sk-proj-AAAAAAAAAAAAAAAA", "sk-proj-AAAA"},
		{"anthropic key", "key sk-ant-api03-ZZZZZZZZZZ rejected", "sk-ant-api03-ZZ"},
		{"jwt", `token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U`, "eyJhbGciOiJIUzI1NiJ9"},
		{"json secret", `{"access_token":"secret-value-12345","email":"a@b.c"}`, "secret-value-12345"},
		{"query param", "GET /x?api_key=abcd1234efgh&page=2", "abcd1234efgh"},
	}
	for _, tc := range cases {
		out := String(tc.in)
		if strings.Contains(out, tc.leak) {
			t.Errorf("%s: credential survived redaction: %q", tc.name, out)
		}
		if !strings.Contains(out, mask) {
			t.Errorf("%s: no mask inserted: %q", tc.name, out)
		}
	}
}

// Redaction must not destroy the parts an operator needs to debug.
func TestStringKeepsContext(t *testing.T) {
	in := "channel 42 test failed: Bearer abcdef1234567890 (HTTP 401)"
	out := String(in)
	for _, want := range []string{"channel 42 test failed", "HTTP 401", "Bearer"} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q from %q", want, out)
		}
	}
}

func TestStringLeavesCleanTextAlone(t *testing.T) {
	in := "channel 7 test ok, latency 380ms, model gemini-2.5-flash"
	if got := String(in); got != in {
		t.Errorf("clean text was modified: %q", got)
	}
}

func TestBytesMatchesString(t *testing.T) {
	in := "Bearer abcdef1234567890"
	if Bytes([]byte(in)) != String(in) {
		t.Error("Bytes and String disagree")
	}
}
