package gemini

import (
	"context"
	"iter"
	"strings"

	"github.com/m2tx/agent_example/internal/agent"
	"github.com/m2tx/agent_example/internal/model"
	"google.golang.org/genai"
)

// Provider implements agent.LLMProvider using the Google Gemini API.
type Provider struct {
	client *genai.Client
	model  string
}

// New creates a new Gemini provider.
func New(client *genai.Client, modelName string) *Provider {
	return &Provider{client: client, model: modelName}
}

func (p *Provider) Send(ctx context.Context, req agent.ProviderRequest) ([]model.Content, error) {
	initialHistory := toGenAIContents(req.History)

	chat, err := p.client.Chats.Create(ctx, p.model, &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemInstruction}},
		},
		Tools: buildTools(req.Tools),
	}, initialHistory)
	if err != nil {
		return nil, err
	}

	resp, err := chat.SendMessage(ctx, genai.Part{Text: req.Prompt})
	if err != nil {
		return nil, err
	}

	if err := processResponse(ctx, chat, resp, req.HandleFunctionCall); err != nil {
		return nil, err
	}

	full := chat.History(true)
	newGenAI := full[len(initialHistory):]
	return toModelContents(newGenAI), nil
}

func (p *Provider) SendStream(ctx context.Context, req agent.ProviderRequest, onText func(string) error, onFunctionCall func(name string, args map[string]any) error, onTurnDone func() error) ([]model.Content, error) {
	initialHistory := toGenAIContents(req.History)

	chat, err := p.client.Chats.Create(ctx, p.model, &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemInstruction}},
		},
		Tools: buildTools(req.Tools),
	}, initialHistory)
	if err != nil {
		return nil, err
	}

	err = processResponseStream(ctx, chat, onText, onFunctionCall, onTurnDone, req.HandleFunctionCall, func() iter.Seq2[*genai.GenerateContentResponse, error] {
		return chat.SendMessageStream(ctx, genai.Part{Text: req.Prompt})
	})
	if err != nil {
		return nil, err
	}

	full := chat.History(true)
	newGenAI := full[len(initialHistory):]
	return toModelContents(newGenAI), nil
}

func buildTools(fns map[string]*agent.FunctionDeclaration) []*genai.Tool {
	decls := make([]*genai.FunctionDeclaration, 0, len(fns))
	for _, fd := range fns {
		decls = append(decls, &genai.FunctionDeclaration{
			Name:                 fd.Name,
			Description:          fd.Description,
			ParametersJsonSchema: resolveRefs(fd.ParametersSchema),
			ResponseJsonSchema:   resolveRefs(fd.ResponseSchema),
		})
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// resolveRefs flattens JSON Schema $defs/$ref references that Gemini does not support.
// It collects $defs from all nesting levels before resolving references.
func resolveRefs(schema any) any {
	m, ok := schema.(map[string]any)
	if !ok {
		return schema
	}

	defs := map[string]any{}
	collectDefs(m, defs)

	var resolve func(node any, visited map[string]bool) any
	resolve = func(node any, visited map[string]bool) any {
		nm, ok := node.(map[string]any)
		if !ok {
			return node
		}

		if ref, ok := nm["$ref"].(string); ok {
			name := strings.TrimPrefix(ref, "#/$defs/")
			if visited[name] {
				return map[string]any{"type": "object"}
			}
			if def, found := defs[name]; found {
				visited[name] = true
				resolved := resolve(def, visited)
				delete(visited, name)
				return resolved
			}
			return nm
		}

		result := make(map[string]any, len(nm))
		for k, v := range nm {
			if k == "$defs" {
				continue
			}
			switch vt := v.(type) {
			case map[string]any:
				result[k] = resolve(vt, visited)
			case []any:
				arr := make([]any, len(vt))
				for i, item := range vt {
					arr[i] = resolve(item, visited)
				}
				result[k] = arr
			default:
				result[k] = v
			}
		}
		return result
	}

	return resolve(m, map[string]bool{})
}

// collectDefs recursively gathers all $defs entries from any level of the schema.
// Outer definitions take precedence over inner ones with the same name.
func collectDefs(node any, out map[string]any) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	if defs, ok := m["$defs"].(map[string]any); ok {
		for k, v := range defs {
			if _, exists := out[k]; !exists {
				out[k] = v
			}
		}
	}
	for k, v := range m {
		if k == "$defs" {
			continue
		}
		switch vt := v.(type) {
		case map[string]any:
			collectDefs(vt, out)
		case []any:
			for _, item := range vt {
				collectDefs(item, out)
			}
		}
	}
}

func processResponse(ctx context.Context, chat *genai.Chat, resp *genai.GenerateContentResponse, handle func(ctx context.Context, name string, args map[string]any) (map[string]any, error)) error {
	var functionResponses []genai.Part

	for _, candidate := range resp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall == nil {
				continue
			}
			funcResp, err := handle(ctx, part.FunctionCall.Name, part.FunctionCall.Args)
			if err != nil {
				return err
			}
			functionResponses = append(functionResponses, genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:       part.FunctionCall.ID,
					Name:     part.FunctionCall.Name,
					Response: funcResp,
				},
			})
		}
	}

	if len(functionResponses) > 0 {
		next, err := chat.SendMessage(ctx, functionResponses...)
		if err != nil {
			return err
		}
		return processResponse(ctx, chat, next, handle)
	}

	return nil
}

func processResponseStream(ctx context.Context, chat *genai.Chat, onText func(string) error, onFunctionCall func(name string, args map[string]any) error, onTurnDone func() error, handle func(ctx context.Context, name string, args map[string]any) (map[string]any, error), streamFn func() iter.Seq2[*genai.GenerateContentResponse, error]) error {
	var pendingCalls []*genai.FunctionCall

	// Phase 1: stream text and collect function calls (without notifying yet).
	for resp, err := range streamFn() {
		if err != nil {
			return err
		}
		for _, candidate := range resp.Candidates {
			if candidate == nil || candidate.Content == nil {
				continue
			}
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					if err := onText(part.Text); err != nil {
						return err
					}
				}
				if part.FunctionCall != nil {
					pendingCalls = append(pendingCalls, part.FunctionCall)
				}
			}
		}
	}

	// Phase 2: LLM turn is done — let the frontend remove the typing indicator.
	if onTurnDone != nil {
		if err := onTurnDone(); err != nil {
			return err
		}
	}

	// Phase 3: notify and execute function calls.
	var functionResponses []genai.Part
	for _, fc := range pendingCalls {
		if onFunctionCall != nil {
			if err := onFunctionCall(fc.Name, fc.Args); err != nil {
				return err
			}
		}
		funcResp, err := handle(ctx, fc.Name, fc.Args)
		if err != nil {
			return err
		}
		functionResponses = append(functionResponses, genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				ID:       fc.ID,
				Name:     fc.Name,
				Response: funcResp,
			},
		})
	}

	if len(functionResponses) > 0 {
		return processResponseStream(ctx, chat, onText, onFunctionCall, onTurnDone, handle, func() iter.Seq2[*genai.GenerateContentResponse, error] {
			return chat.SendMessageStream(ctx, functionResponses...)
		})
	}

	return nil
}

func toModelContents(contents []*genai.Content) []model.Content {
	result := make([]model.Content, 0, len(contents))
	for _, c := range contents {
		mc := model.Content{Role: c.Role, Parts: make([]model.Part, 0, len(c.Parts))}
		for _, p := range c.Parts {
			mp := model.Part{Text: p.Text}
			if p.FunctionCall != nil {
				mp.FunctionCall = &model.FunctionCall{
					ID:   p.FunctionCall.ID,
					Name: p.FunctionCall.Name,
					Args: p.FunctionCall.Args,
				}
			}
			if p.FunctionResponse != nil {
				mp.FunctionResponse = &model.FunctionResponse{
					ID:       p.FunctionResponse.ID,
					Name:     p.FunctionResponse.Name,
					Response: p.FunctionResponse.Response,
				}
			}
			mc.Parts = append(mc.Parts, mp)
		}
		result = append(result, mc)
	}
	return result
}

func toGenAIContents(contents []model.Content) []*genai.Content {
	result := make([]*genai.Content, 0, len(contents))
	for _, c := range contents {
		gc := &genai.Content{Role: c.Role, Parts: make([]*genai.Part, 0, len(c.Parts))}
		for _, p := range c.Parts {
			gp := &genai.Part{Text: p.Text}
			if p.FunctionCall != nil {
				gp.FunctionCall = &genai.FunctionCall{
					ID:   p.FunctionCall.ID,
					Name: p.FunctionCall.Name,
					Args: p.FunctionCall.Args,
				}
			}
			if p.FunctionResponse != nil {
				gp.FunctionResponse = &genai.FunctionResponse{
					ID:       p.FunctionResponse.ID,
					Name:     p.FunctionResponse.Name,
					Response: p.FunctionResponse.Response,
				}
			}
			gc.Parts = append(gc.Parts, gp)
		}
		result = append(result, gc)
	}
	return result
}

// Ensure the interface is satisfied at compile time.
var _ agent.LLMProvider = (*Provider)(nil)
