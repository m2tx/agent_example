package agent

import (
	"context"
	"fmt"

	"github.com/m2tx/agent_example/internal/model"
	"github.com/m2tx/agent_example/internal/repository"
	"google.golang.org/genai"
)

type Agent struct {
	client            *genai.Client
	model             string
	systemInstruction string
	functionsMap      map[string]*FunctionDeclaration
	sessionRepository repository.SessionRepository
}

type FunctionDeclaration struct {
	Name             string
	Description      string
	ParametersSchema any
	ResponseSchema   any
	FunctionCall     FunctionCallFn
}

type FunctionCallFn func(ctx context.Context, args map[string]any) (map[string]any, error)

func New(client *genai.Client, model string, systemInstruction string) *Agent {
	return &Agent{
		client:            client,
		model:             model,
		systemInstruction: systemInstruction,
		functionsMap:      make(map[string]*FunctionDeclaration),
	}
}

func NewWithRepo(client *genai.Client, model string, systemInstruction string, sessionRepository repository.SessionRepository) *Agent {
	a := New(client, model, systemInstruction)
	a.sessionRepository = sessionRepository
	return a
}

func (a *Agent) AddFunctionCall(functionDeclaration *FunctionDeclaration) error {
	if functionDeclaration == nil {
		return fmt.Errorf("function declaration cannot be nil")
	}

	if functionDeclaration.Name == "" {
		return fmt.Errorf("function name cannot be empty")
	}

	if functionDeclaration.FunctionCall == nil {
		return fmt.Errorf("function call implementation cannot be nil")
	}

	a.functionsMap[functionDeclaration.Name] = functionDeclaration

	return nil
}

func (a *Agent) getTools() []*genai.Tool {
	functions := []*genai.FunctionDeclaration{}

	for _, fd := range a.functionsMap {
		functions = append(functions, &genai.FunctionDeclaration{
			Name:                 fd.Name,
			Description:          fd.Description,
			ParametersJsonSchema: fd.ParametersSchema,
			ResponseJsonSchema:   fd.ResponseSchema,
		})
	}

	return []*genai.Tool{
		{
			FunctionDeclarations: functions,
		},
	}
}

func (a *Agent) getChat(ctx context.Context, sessionID string) (*genai.Chat, error) {
	initialHistory := []*genai.Content{}
	if a.sessionRepository != nil {
		stored, err := a.sessionRepository.Load(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("getChat: load history: %w", err)
		}
		if stored != nil {
			initialHistory = toGenAIContents(stored)
		}
	}

	chat, err := a.client.Chats.Create(ctx, a.model, &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: a.systemInstruction}},
		},
		Tools: a.getTools(),
	}, initialHistory)
	if err != nil {
		return nil, err
	}

	return chat, nil
}

func (a *Agent) Send(ctx context.Context, sessionID string, prompt string) ([]model.Content, error) {
	chat, err := a.getChat(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	resp, err := chat.SendMessage(ctx, genai.Part{
		Text: prompt,
	})
	if err != nil {
		return nil, err
	}

	contents, err := a.processResponse(ctx, chat, resp)
	if err != nil {
		return nil, err
	}

	if a.sessionRepository != nil {
		if saveErr := a.sessionRepository.Save(ctx, sessionID, toModelContents(chat.History(true))); saveErr != nil {
			fmt.Printf("agent: warning: failed to save session %q: %v\n", sessionID, saveErr)
		}
	}

	return parseContents(contents)
}

func (a *Agent) ClearSession(ctx context.Context, sessionID string) {
	if a.sessionRepository != nil {
		if err := a.sessionRepository.Delete(ctx, sessionID); err != nil {
			fmt.Printf("agent: warning: failed to delete session %q: %v\n", sessionID, err)
		}
	}
}

func (a *Agent) GetSession(ctx context.Context, sessionID string) ([]model.Content, error) {
	if a.sessionRepository == nil {
		return []model.Content{}, nil
	}

	stored, err := a.sessionRepository.Load(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("GetSession: %w", err)
	}

	if stored == nil {
		return []model.Content{}, nil
	}

	return stored, nil
}

func (a *Agent) handleFunctionCall(ctx context.Context, functionName string, args map[string]any) (map[string]any, error) {
	if fd, exists := a.functionsMap[functionName]; exists {
		return fd.FunctionCall(ctx, args)
	}

	return nil, fmt.Errorf("function %s not found", functionName)
}

func (a *Agent) processResponse(ctx context.Context, chat *genai.Chat, resp *genai.GenerateContentResponse) ([]*genai.Content, error) {
	contents := []*genai.Content{}
	functionResponses := []genai.Part{}

	for _, candidate := range resp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}

		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				funcResp, err := a.handleFunctionCall(ctx, part.FunctionCall.Name, part.FunctionCall.Args)
				if err != nil {
					return nil, err
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

		contents = append(contents, candidate.Content)
	}

	if len(functionResponses) > 0 {
		resp, err := chat.SendMessage(ctx, functionResponses...)
		if err != nil {
			return nil, err
		}

		fContents, err := a.processResponse(ctx, chat, resp)
		if err != nil {
			return nil, err
		}

		contents = append(contents, fContents...)
	}

	return contents, nil
}

// parseContents converts a slice of genai.Content into []model.Content for the API response.
func parseContents(contents []*genai.Content) ([]model.Content, error) {
	parsed := make([]model.Content, 0, len(contents))
	for _, c := range contents {
		parts, err := parseParts(c.Parts)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, model.Content{Parts: parts, Role: c.Role})
	}
	return parsed, nil
}

func parseParts(parts []*genai.Part) ([]model.Part, error) {
	parsed := make([]model.Part, 0, len(parts))
	for _, p := range parts {
		switch {
		case p.FunctionCall != nil:
			parsed = append(parsed, model.Part{
				FunctionCall: &model.FunctionCall{
					ID:   p.FunctionCall.ID,
					Name: p.FunctionCall.Name,
					Args: p.FunctionCall.Args,
				},
			})
		case p.FunctionResponse != nil:
			parsed = append(parsed, model.Part{
				FunctionResponse: &model.FunctionResponse{
					ID:       p.FunctionResponse.ID,
					Name:     p.FunctionResponse.Name,
					Response: p.FunctionResponse.Response,
				},
			})
		default:
			parsed = append(parsed, model.Part{Text: p.Text})
		}
	}
	return parsed, nil
}

// toModelContents converts genai history to []model.Content for persistence.
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

// toGenAIContents converts []model.Content from persistence back to genai history.
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
