package tools

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/Ruanyuxi1337/newapi-operator/internal/client"
)

// result is the uniform JSON envelope every tool returns.
type result struct {
	HTTPStatus int    `json:"http_status"`
	Message    string `json:"message,omitempty"`
	Data       any    `json:"data,omitempty"`
}

// call performs an admin request and decodes the payload.
func (r *Registry) call(ctx context.Context, method, path string, q url.Values, body any) (any, error) {
	resp, err := r.client.Do(ctx, method, path, q, body)
	if err != nil {
		return nil, err
	}
	out := result{HTTPStatus: resp.StatusCode}
	if resp.Envelope != nil {
		out.Message = resp.Envelope.Message
	}
	data := resp.Data()
	if len(data) > 0 && string(data) != "null" {
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			// Not JSON: hand back the text so the agent can still read it.
			out.Data = strings.TrimSpace(string(data))
		} else {
			out.Data = v
		}
	}
	return out, nil
}

// callRaw performs a request and returns the response untouched, for tools that
// need the headers or the raw body.
func (r *Registry) callRaw(ctx context.Context, method, path string, q url.Values, body any) (*client.Response, error) {
	return r.client.Do(ctx, method, path, q, body)
}

// pageQuery builds the standard pagination query used by list endpoints.
func pageQuery(a Args) url.Values {
	q := url.Values{}
	if n, ok := argInt(a, "p"); ok {
		q.Set("p", strconv.Itoa(n))
	}
	if n, ok := argInt(a, "page_size"); ok {
		q.Set("page_size", strconv.Itoa(n))
	}
	return q
}

// stringList flattens a JSON value that is either a comma-separated string or an
// array of strings into a slice.
func stringList(v any) ([]string, bool) {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return nil, true
		}
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out, true
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out, true
	}
	return nil, false
}

// joinModels renders a model list as the comma-separated string New API stores.
func joinModels(v any) (string, bool) {
	list, ok := stringList(v)
	if !ok {
		return "", false
	}
	return strings.Join(list, ","), true
}

// jsonString encodes a value as a JSON string, because New API stores
// model_mapping and several setting fields as stringified JSON.
func jsonString(v any) (string, bool) {
	if s, ok := v.(string); ok {
		return s, true
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

// mergeExtra copies caller-supplied raw fields into a request body.
func mergeExtra(body map[string]any, a Args) {
	extra, ok := a["extra"]
	if !ok {
		return
	}
	m, ok := extra.(map[string]any)
	if !ok {
		return
	}
	for k, v := range m {
		body[k] = v
	}
}
