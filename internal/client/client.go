// Package client is a thin, strict HTTP client for the New API admin surface.
//
// It exists instead of a generated OpenAPI client because the admin API is not
// fully described by any spec: the two behaviours that break naive clients are
// handled here explicitly.
//
//  1. New API answers a failed call with HTTP 200 and {"success": false}.
//     Checking the status code alone reports success on every rejection.
//  2. The admin surface needs both an Authorization bearer token and a
//     New-Api-User header carrying the numeric user id.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Ruanyuxi1337/newapi-operator/internal/redact"
)

// ErrReadOnly is returned for a mutating call while the server is in read-only mode.
var ErrReadOnly = errors.New("server is in read-only mode: NEWAPI_READ_ONLY=true")

// Sentinel errors surfaced to the agent so it can react without parsing prose.
var (
	ErrNotFound   = errors.New("not found (404)")
	ErrUnauth     = errors.New("unauthorized (401/403): the access token is invalid, expired, or missing a scope")
	ErrRateLimit  = errors.New("rate limited (429)")
	ErrUpstream   = errors.New("upstream error (5xx)")
	ErrBadRequest = errors.New("bad request (400)")
)

// Envelope is New API's universal JSON response wrapper.
type Envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Response is one completed upstream call.
type Response struct {
	StatusCode int
	Body       []byte
	// Envelope is non-nil when the body parsed as New API's wrapper.
	Envelope *Envelope
}

// APIError describes a rejected call.
type APIError struct {
	StatusCode int
	Message    string
	Sentinel   error
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("%s (HTTP %d)", e.Sentinel, e.StatusCode)
	}
	return fmt.Sprintf("%s (HTTP %d): %s", e.Sentinel, e.StatusCode, e.Message)
}

func (e *APIError) Unwrap() error { return e.Sentinel }

// Client talks to one New API instance.
type Client struct {
	baseURL  string
	token    string
	userID   string
	http     *http.Client
	readOnly bool
}

// Options configures NewWithOptions.
type Options struct {
	BaseURL  string
	Token    string
	UserID   string
	HTTP     *http.Client
	ReadOnly bool
}

// NewWithOptions builds a client from explicit options.
func NewWithOptions(o Options) *Client {
	if o.HTTP == nil {
		o.HTTP = &http.Client{}
	}
	return &Client{
		baseURL:  strings.TrimSuffix(o.BaseURL, "/"),
		token:    o.Token,
		userID:   o.UserID,
		http:     o.HTTP,
		readOnly: o.ReadOnly,
	}
}

// ReadOnly reports whether mutating calls are blocked.
func (c *Client) ReadOnly() bool { return c.readOnly }

// BaseURL returns the configured origin.
func (c *Client) BaseURL() string { return c.baseURL }

// Do performs one admin call.
//
// path must start with "/api/". query may be nil. body may be nil, a
// json.RawMessage, or any value that encodes to JSON.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any) (*Response, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	// Path allowlist: admin calls only, and no way to climb out of /api/.
	// net/http sends the path verbatim, so a server that normalises dot
	// segments could otherwise route /api/../v1/... to the relay surface.
	if !validAdminPath(path) {
		return nil, fmt.Errorf("refusing to call %q: admin calls must target /api/*", path)
	}
	if c.readOnly && isMutating(method) {
		return nil, ErrReadOnly
	}

	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case json.RawMessage:
		if len(b) > 0 {
			reader = bytes.NewReader(b)
		}
	case []byte:
		if len(b) > 0 {
			reader = bytes.NewReader(b)
		}
	default:
		encoded, err := json.Marshal(b)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("New-Api-User", c.userID)
	req.Header.Set("Accept", "application/json")
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// net/http errors embed the full URL; it never contains a credential,
		// but redact defensively anyway.
		return nil, fmt.Errorf("request failed: %w", errors.New(redact.String(err.Error())))
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	out := &Response{StatusCode: resp.StatusCode, Body: raw}
	var env Envelope
	if len(raw) > 0 && json.Unmarshal(raw, &env) == nil && (env.Success || env.Message != "" || env.Data != nil) {
		out.Envelope = &env
	}

	if err := classify(out); err != nil {
		return out, err
	}
	return out, nil
}

// classify maps a completed response onto a sentinel error when the call failed.
func classify(r *Response) error {
	// New API's hidden failure mode: 200 OK plus success:false.
	if r.Envelope != nil && !r.Envelope.Success {
		return &APIError{
			StatusCode: r.StatusCode,
			Message:    redact.String(r.Envelope.Message),
			Sentinel:   sentinelFor(r.StatusCode),
		}
	}
	if r.StatusCode >= 200 && r.StatusCode < 300 {
		return nil
	}
	return &APIError{
		StatusCode: r.StatusCode,
		Message:    redact.String(truncate(string(r.Body), 512)),
		Sentinel:   sentinelFor(r.StatusCode),
	}
}

func sentinelFor(status int) error {
	switch {
	case status == 400:
		return ErrBadRequest
	case status == 401 || status == 403:
		return ErrUnauth
	case status == 404:
		return ErrNotFound
	case status == 429:
		return ErrRateLimit
	case status >= 500:
		return ErrUpstream
	default:
		return fmt.Errorf("unexpected status %d", status)
	}
}

// validAdminPath reports whether p is an admin API path that cannot escape
// /api/. Reject raw CR/LF, backslashes, repeated slashes, dot segments and
// percent-encoded traversal before anything reaches net/http.
func validAdminPath(p string) bool {
	if p != "/api" && !strings.HasPrefix(p, "/api/") {
		return false
	}
	if strings.ContainsAny(p, "\\\r\n	 ?#") || strings.Contains(p, "//") {
		return false
	}
	lower := strings.ToLower(p)
	if strings.Contains(lower, "%2e") || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func isMutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Data returns the envelope's data payload, or the raw body when there is no envelope.
func (r *Response) Data() json.RawMessage {
	if r.Envelope != nil && len(r.Envelope.Data) > 0 {
		return r.Envelope.Data
	}
	return r.Body
}

// Decode unmarshals the response payload into v.
func (r *Response) Decode(v any) error {
	data := r.Data()
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, v)
}
