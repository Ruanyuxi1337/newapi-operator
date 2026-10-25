// Package tools defines the MCP tool surface exposed to the agent.
//
// Every tool is declared as data (name, description, JSON Schema, handler) and
// adapted to the MCP SDK at registration time. That keeps the 40-odd tools
// uniform: one place enforces read-only mode, argument decoding, JSON result
// encoding, and credential redaction.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ruanyuxi1337/newapi-operator/internal/client"
)

// Args is the decoded tool argument map.
type Args map[string]any

// Handler implements one tool. Returning an error surfaces as a protocol error;
// returning a value encodes it as the tool's JSON result.
type Handler func(ctx context.Context, a Args) (any, error)

// Tool is one MCP tool definition.
type Tool struct {
	Name        string
	Title       string
	Description string
	Schema      map[string]any
	// Mutating marks write operations; the server refuses them in read-only mode.
	Mutating bool
	Handler  Handler
}

// Registry holds the tool set and the shared upstream client.
type Registry struct {
	client       *client.Client
	quotaPerUnit float64
	tools        []Tool
}

// NewRegistry builds an empty registry bound to c.
func NewRegistry(c *client.Client, quotaPerUnit float64) *Registry {
	if quotaPerUnit <= 0 {
		quotaPerUnit = 500000
	}
	return &Registry{client: c, quotaPerUnit: quotaPerUnit}
}

// Add appends tools.
func (r *Registry) Add(ts ...Tool) { r.tools = append(r.tools, ts...) }

// Tools returns the registered tools.
func (r *Registry) Tools() []Tool { return r.tools }

// Attach registers every tool with the MCP server.
func (r *Registry) Attach(s *mcp.Server) {
	for _, t := range r.tools {
		tool := &mcp.Tool{
			Name:        t.Name,
			Title:       t.Title,
			Description: t.Description,
			InputSchema: t.Schema,
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint: !t.Mutating,
			},
		}
		s.AddTool(tool, r.adapt(t))
	}
}

func (r *Registry) adapt(t Tool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args Args
		if len(req.Params.Arguments) > 0 && string(req.Params.Arguments) != "null" {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errorResult(fmt.Errorf("invalid arguments: %w", err)), nil
			}
		}
		if args == nil {
			args = Args{}
		}
		if t.Mutating && r.client.ReadOnly() {
			return errorResult(fmt.Errorf("%s is a mutating tool; the server runs with NEWAPI_READ_ONLY=true", t.Name)), nil
		}
		out, err := t.Handler(ctx, args)
		if err != nil {
			return errorResult(err), nil
		}
		return jsonResult(out)
	}
}

func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	if v == nil {
		v = map[string]any{"ok": true}
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult(fmt.Errorf("encode result: %w", err)), nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
	}, nil
}

// ---- schema helpers -------------------------------------------------------

// object builds a JSON Schema object node.
func object(desc string, props map[string]any, required ...string) map[string]any {
	s := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if desc != "" {
		s["description"] = desc
	}
	s["additionalProperties"] = false
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func numProp(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func arrProp(desc string, items map[string]any) map[string]any {
	s := map[string]any{"type": "array", "description": desc}
	if items != nil {
		s["items"] = items
	}
	return s
}

func enumProp(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

func strArrayProp(desc string) map[string]any {
	return arrProp(desc, map[string]any{"type": "string"})
}

func intArrayProp(desc string) map[string]any {
	return arrProp(desc, map[string]any{"type": "integer"})
}

// ---- argument accessors ---------------------------------------------------

func argString(a Args, key string) string {
	if v, ok := a[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func argInt(a Args, key string) (int, bool) {
	v, ok := a[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	}
	return 0, false
}

func argBool(a Args, key string) (bool, bool) {
	if v, ok := a[key]; ok {
		if b, ok := v.(bool); ok {
			return b, true
		}
	}
	return false, false
}

// argRaw returns the value under key already marshalled as JSON, for pass-through fields.
func argRaw(a Args, key string) (json.RawMessage, bool) {
	v, ok := a[key]
	if !ok {
		return nil, false
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return raw, true
}

// requireInt reads a required integer argument.
func requireInt(a Args, key string) (int, error) {
	n, ok := argInt(a, key)
	if !ok {
		return 0, fmt.Errorf("missing required integer argument %q", key)
	}
	return n, nil
}

// requireString reads a required non-empty string argument.
func requireString(a Args, key string) (string, error) {
	s := argString(a, key)
	if s == "" {
		return "", fmt.Errorf("missing required string argument %q", key)
	}
	return s, nil
}
