package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/m2tx/agent_example/internal/agent"
	"github.com/m2tx/agent_example/internal/model"
)

// Provider implements agent.LLMProvider using the Anthropic API.
type Provider struct {
	client    anthropic.Client
	modelName string
}

// New creates a new Anthropic provider.
func New(apiKey string, modelName string) *Provider {
	return &Provider{
		client:    anthropic.NewClient(option.WithAPIKey(apiKey)),
		modelName: modelName,
	}
}

func (p *Provider) Send(ctx context.Context, req agent.ProviderRequest) ([]model.Content, error) {
	messages := historyToMessages(req.History)
	tools := buildTools(req.Tools)

	userMsg := anthropic.NewUserMessage(anthropic.NewTextBlock(req.Prompt))
	messages = append(messages, userMsg)

	var newContents []model.Content
	newContents = append(newContents, model.Content{
		Role:  "user",
		Parts: []model.Part{{Text: req.Prompt}},
	})

	for {
		resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(p.modelName),
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{
				{Text: req.SystemInstruction},
			},
			Tools:    tools,
			Messages: messages,
		})
		if err != nil {
			return nil, err
		}

		modelContent := responseToModelContent(resp)
		newContents = append(newContents, modelContent)

		if resp.StopReason != anthropic.StopReasonToolUse {
			break
		}

		toolResults, toolResultContent := processToolUse(ctx, resp, req.HandleFunctionCall)
		messages = append(messages, anthropic.NewAssistantMessage(responseToContentParams(resp)...))
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
		newContents = append(newContents, toolResultContent)
	}

	return newContents, nil
}

func (p *Provider) SendStream(ctx context.Context, req agent.ProviderRequest, onText func(string) error, onFunctionCall func(name string, args map[string]any) error) ([]model.Content, error) {
	messages := historyToMessages(req.History)
	tools := buildTools(req.Tools)

	userMsg := anthropic.NewUserMessage(anthropic.NewTextBlock(req.Prompt))
	messages = append(messages, userMsg)

	var newContents []model.Content
	newContents = append(newContents, model.Content{
		Role:  "user",
		Parts: []model.Part{{Text: req.Prompt}},
	})

	for {
		stream := p.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(p.modelName),
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{
				{Text: req.SystemInstruction},
			},
			Tools:    tools,
			Messages: messages,
		})

		acc := anthropic.Message{}
		for stream.Next() {
			event := stream.Current()
			if err := acc.Accumulate(event); err != nil {
				return nil, err
			}
			if event.Type == "content_block_delta" {
				delta := event.AsContentBlockDelta()
				if delta.Delta.Type == "text_delta" {
					if err := onText(delta.Delta.Text); err != nil {
						return nil, err
					}
				}
			}
		}
		if err := stream.Err(); err != nil {
			return nil, err
		}

		// notify about any function calls
		if onFunctionCall != nil {
			for _, block := range acc.Content {
				if block.Type == "tool_use" {
					tb := block.AsToolUse()
					var args map[string]any
					_ = json.Unmarshal(tb.Input, &args)
					if err := onFunctionCall(tb.Name, args); err != nil {
						return nil, err
					}
				}
			}
		}

		modelContent := responseToModelContent(&acc)
		newContents = append(newContents, modelContent)

		if acc.StopReason != anthropic.StopReasonToolUse {
			break
		}

		toolResults, toolResultContent := processToolUse(ctx, &acc, req.HandleFunctionCall)
		messages = append(messages, anthropic.NewAssistantMessage(responseToContentParams(&acc)...))
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
		newContents = append(newContents, toolResultContent)
	}

	return newContents, nil
}

// historyToMessages converts stored model.Content history to Anthropic MessageParam slice.
// Role "model" is mapped to "assistant"; "user" is kept as-is.
func historyToMessages(history []model.Content) []anthropic.MessageParam {
	msgs := make([]anthropic.MessageParam, 0, len(history))
	for _, c := range history {
		blocks := partsToContentBlocks(c.Parts)
		if len(blocks) == 0 {
			continue
		}
		switch c.Role {
		case "model":
			msgs = append(msgs, anthropic.NewAssistantMessage(blocks...))
		default:
			msgs = append(msgs, anthropic.NewUserMessage(blocks...))
		}
	}
	return msgs
}

func partsToContentBlocks(parts []model.Part) []anthropic.ContentBlockParamUnion {
	var blocks []anthropic.ContentBlockParamUnion
	for _, p := range parts {
		switch {
		case p.FunctionCall != nil:
			args := p.FunctionCall.Args
			if args == nil {
				args = map[string]any{}
			}
			inputJSON, _ := json.Marshal(args)
			blocks = append(blocks, anthropic.NewToolUseBlock(p.FunctionCall.ID, json.RawMessage(inputJSON), p.FunctionCall.Name))
		case p.FunctionResponse != nil:
			respJSON, _ := json.Marshal(p.FunctionResponse.Response)
			blocks = append(blocks, anthropic.NewToolResultBlock(p.FunctionResponse.ID, string(respJSON), false))
		case p.Text != "":
			blocks = append(blocks, anthropic.NewTextBlock(p.Text))
		}
	}
	return blocks
}

func buildTools(fns map[string]*agent.FunctionDeclaration) []anthropic.ToolUnionParam {
	tools := make([]anthropic.ToolUnionParam, 0, len(fns))
	for _, fd := range fns {
		schema := extractSchema(fd.ParametersSchema)
		tool := anthropic.ToolParam{
			Name:        fd.Name,
			Description: anthropic.String(fd.Description),
			InputSchema: schema,
		}
		tools = append(tools, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return tools
}

func extractSchema(parametersSchema any) anthropic.ToolInputSchemaParam {
	// Always set Properties to a non-nil value so the struct is not considered
	// zero by omitzero — the API requires input_schema even for tools with no parameters.
	schema := anthropic.ToolInputSchemaParam{
		Properties: map[string]any{},
	}
	m, ok := parametersSchema.(map[string]any)
	if !ok {
		return schema
	}
	if props, ok := m["properties"]; ok {
		schema.Properties = props
	}
	if req, ok := m["required"].([]string); ok {
		schema.Required = req
	} else if req, ok := m["required"].([]any); ok {
		strs := make([]string, 0, len(req))
		for _, r := range req {
			if s, ok := r.(string); ok {
				strs = append(strs, s)
			}
		}
		schema.Required = strs
	}
	return schema
}

func responseToModelContent(resp *anthropic.Message) model.Content {
	mc := model.Content{Role: "model"}
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			tb := block.AsText()
			mc.Parts = append(mc.Parts, model.Part{Text: tb.Text})
		case "tool_use":
			tb := block.AsToolUse()
			var args map[string]any
			_ = json.Unmarshal(tb.Input, &args)
			mc.Parts = append(mc.Parts, model.Part{
				FunctionCall: &model.FunctionCall{
					ID:   tb.ID,
					Name: tb.Name,
					Args: args,
				},
			})
		}
	}
	return mc
}

func responseToContentParams(resp *anthropic.Message) []anthropic.ContentBlockParamUnion {
	var blocks []anthropic.ContentBlockParamUnion
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			tb := block.AsText()
			blocks = append(blocks, anthropic.NewTextBlock(tb.Text))
		case "tool_use":
			tb := block.AsToolUse()
			blocks = append(blocks, anthropic.NewToolUseBlock(tb.ID, tb.Input, tb.Name))
		}
	}
	return blocks
}

func processToolUse(ctx context.Context, resp *anthropic.Message, handle func(ctx context.Context, name string, args map[string]any) (map[string]any, error)) ([]anthropic.ContentBlockParamUnion, model.Content) {
	var toolResults []anthropic.ContentBlockParamUnion
	userContent := model.Content{Role: "user"}

	for _, block := range resp.Content {
		if block.Type != "tool_use" {
			continue
		}
		tb := block.AsToolUse()
		var args map[string]any
		_ = json.Unmarshal(tb.Input, &args)

		result, err := handle(ctx, tb.Name, args)
		var resultStr string
		if err != nil {
			resultStr = fmt.Sprintf(`{"error": %q}`, err.Error())
		} else {
			b, _ := json.Marshal(result)
			resultStr = string(b)
		}

		toolResults = append(toolResults, anthropic.NewToolResultBlock(tb.ID, resultStr, err != nil))
		userContent.Parts = append(userContent.Parts, model.Part{
			FunctionResponse: &model.FunctionResponse{
				ID:       tb.ID,
				Name:     tb.Name,
				Response: result,
			},
		})
	}

	return toolResults, userContent
}

// Ensure the interface is satisfied at compile time.
var _ agent.LLMProvider = (*Provider)(nil)
