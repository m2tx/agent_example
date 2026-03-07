package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/m2tx/agent_example/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolRegistry is implemented by any type that can accept tool registrations.
type ToolRegistry interface {
	AddFunctionCall(*agent.FunctionDeclaration) error
}

// Client wraps an MCP client session.
type Client struct {
	session *mcp.ClientSession
}

// NewClient creates and connects an MCP client using either a stdio command or an HTTP endpoint.
// Exactly one of command or endpoint must be non-empty.
// env is an optional list of "KEY=VALUE" strings injected into the server process environment
// (merged on top of the current process environment). Ignored for HTTP transports.
func NewClient(ctx context.Context, command, endpoint string, env []string) (*Client, error) {
	var transport mcp.Transport
	switch {
	case command != "":
		parts := strings.Fields(command)
		cmd := exec.Command(parts[0], parts[1:]...)
		if len(env) > 0 {
			cmd.Env = append(os.Environ(), env...)
		}
		transport = &mcp.CommandTransport{Command: cmd}
	case endpoint != "":
		transport = &mcp.StreamableClientTransport{Endpoint: endpoint}
	default:
		return nil, fmt.Errorf("mcp: either command or endpoint must be set")
	}

	c := mcp.NewClient(&mcp.Implementation{Name: "agent_example", Version: "1.0.0"}, nil)
	session, err := c.Connect(ctx, transport, nil)
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
				res, err := c.session.CallTool(ctx, &mcp.CallToolParams{
					Name:      tool.Name,
					Arguments: args,
				})
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

func extractText(contents []mcp.Content) string {
	var parts []string
	for _, c := range contents {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}
