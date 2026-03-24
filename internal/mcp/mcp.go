package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/m2tx/agent_example/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type TransportType string

const (
	TransportSSE        TransportType = "sse"
	TransportStreamable TransportType = "streamable"
)

// ToolRegistry is implemented by any type that can accept tool registrations.
type ToolRegistry interface {
	AddFunctionCall(*agent.FunctionDeclaration) error
}

// Client wraps an MCP client session.
type Client struct {
	session *mcp.ClientSession
}

// NewClient creates and connects an MCP client using an HTTP endpoint.
// The transportType selects the transport: "streamable" or "sse" (default).
func NewClient(ctx context.Context, endpoint string, transportType TransportType) (*Client, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("mcp: endpoint must be set")
	}

	var transport mcp.Transport
	switch transportType {
	case TransportSSE:
		transport = &mcp.SSEClientTransport{Endpoint: endpoint}
	default:
		transport = &mcp.StreamableClientTransport{Endpoint: endpoint}
	}

	c := mcp.NewClient(&mcp.Implementation{Name: "agent_example", Version: "1.0.0"}, nil)
	session, err := c.Connect(ctx, transport, &mcp.ClientSessionOptions{})
	if err != nil {
		return nil, fmt.Errorf("mcp connect: %w", err)
	}

	return &Client{session: session}, nil
}

// Close terminates the underlying MCP session.
func (c *Client) Close() {
	c.session.Close()
}

// ListTools fetches all tools from the MCP server and returns them as FunctionDeclarations.
func (c *Client) ListTools(ctx context.Context) ([]*agent.FunctionDeclaration, error) {
	result, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp list tools: %w", err)
	}

	decls := make([]*agent.FunctionDeclaration, 0, len(result.Tools))
	for _, tool := range result.Tools {
		decls = append(decls, &agent.FunctionDeclaration{
			Name:             tool.Name,
			Description:      tool.Description,
			ParametersSchema: tool.InputSchema,
			FunctionCall: func(ctx context.Context, args map[string]any) (map[string]any, error) {
				params := &mcp.CallToolParams{
					Name:      tool.Name,
					Arguments: args,
				}
				if sessionID, ok := agent.SessionIDFromContext(ctx); ok {
					params.Meta = mcp.Meta{"session_id": sessionID}
				}
				res, err := c.session.CallTool(ctx, params)
				if err != nil {
					return nil, fmt.Errorf("mcp call %s: %w", tool.Name, err)
				}
				if res.IsError {
					return nil, fmt.Errorf("tool %s: %s", tool.Name, extractText(res.Content))
				}
				return map[string]any{"result": extractText(res.Content)}, nil
			},
		})
	}

	return decls, nil
}

// RegisterTools fetches all tools from the MCP server and registers them with the given registry.
func (c *Client) RegisterTools(ctx context.Context, registry ToolRegistry) error {
	decls, err := c.ListTools(ctx)
	if err != nil {
		return err
	}

	for _, decl := range decls {
		if err := registry.AddFunctionCall(decl); err != nil {
			return fmt.Errorf("register tool %q: %w", decl.Name, err)
		}
	}

	return nil
}

// RegisterPrompts fetches all prompts from the MCP server and registers each one
// as a callable tool. When invoked, the tool calls GetPrompt with the supplied
// arguments and returns the rendered messages as a single text result.
func (c *Client) RegisterPrompts(ctx context.Context, registry ToolRegistry) error {
	prompts, err := c.ListPrompts(ctx)
	if err != nil {
		return err
	}

	for _, p := range prompts {
		decl := &agent.FunctionDeclaration{
			Name:             p.Name,
			Description:      p.Description,
			ParametersSchema: buildPromptSchema(p),
			FunctionCall: func(ctx context.Context, args map[string]any) (map[string]any, error) {
				strArgs := make(map[string]string, len(args))
				for k, v := range args {
					strArgs[k] = fmt.Sprintf("%v", v)
				}
				result, err := c.GetPrompt(ctx, p.Name, strArgs)
				if err != nil {
					return nil, err
				}
				var parts []string
				for _, msg := range result.Messages {
					parts = append(parts, fmt.Sprintf("[%s] %s", msg.Role, extractText([]mcp.Content{msg.Content})))
				}
				return map[string]any{"messages": strings.Join(parts, "\n")}, nil
			},
		}
		if err := registry.AddFunctionCall(decl); err != nil {
			return fmt.Errorf("register prompt %q: %w", p.Name, err)
		}
	}

	return nil
}

// buildPromptSchema converts MCP prompt arguments into a JSON Schema map.
func buildPromptSchema(p *mcp.Prompt) map[string]any {
	properties := map[string]any{}
	required := []string{}

	for _, arg := range p.Arguments {
		prop := map[string]any{"type": "string"}
		if arg.Description != "" {
			prop["description"] = arg.Description
		}
		properties[arg.Name] = prop
		if arg.Required {
			required = append(required, arg.Name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// ListPrompts fetches all prompts available on the MCP server.
func (c *Client) ListPrompts(ctx context.Context) ([]*mcp.Prompt, error) {
	result, err := c.session.ListPrompts(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp list prompts: %w", err)
	}

	return result.Prompts, nil
}

// GetPrompt retrieves a specific prompt by name, optionally passing arguments
// for template substitution. Returns the resolved messages from the MCP server.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*mcp.GetPromptResult, error) {
	result, err := c.session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp get prompt %q: %w", name, err)
	}

	return result, nil
}

func extractText(contents []mcp.Content) string {
	var parts []string
	for _, c := range contents {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}
