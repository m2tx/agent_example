package gemini

import (
	"context"
	"iter"

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

func (p *Provider) SendStream(ctx context.Context, req agent.ProviderRequest, onText func(string) error, onFunctionCall func(name string, args map[string]any) error) ([]model.Content, error) {
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

	err = processResponseStream(ctx, chat, onText, onFunctionCall, req.HandleFunctionCall, func() iter.Seq2[*genai.GenerateContentResponse, error] {
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
			ParametersJsonSchema: fd.ParametersSchema,
			ResponseJsonSchema:   fd.ResponseSchema,
		})
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
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

func processResponseStream(ctx context.Context, chat *genai.Chat, onText func(string) error, onFunctionCall func(name string, args map[string]any) error, handle func(ctx context.Context, name string, args map[string]any) (map[string]any, error), streamFn func() iter.Seq2[*genai.GenerateContentResponse, error]) error {
	var functionResponses []genai.Part

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
					if onFunctionCall != nil {
						if err := onFunctionCall(part.FunctionCall.Name, part.FunctionCall.Args); err != nil {
							return err
						}
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
		}
	}

	if len(functionResponses) > 0 {
		return processResponseStream(ctx, chat, onText, onFunctionCall, handle, func() iter.Seq2[*genai.GenerateContentResponse, error] {
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
