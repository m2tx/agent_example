package agent

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

type Part struct {
	Text             string `json:"text,omitempty"`
	FunctionCall     string `json:"function_call,omitempty"`
	FunctionResponse string `json:"function_response,omitempty"`
}

type Content struct {
	Parts []Part `json:"parts"`
	Role  string `json:"role"`
}

type Agent struct {
	client            *genai.Client
	model             string
	systemInstruction string
	functionsMap      map[string]*FunctionDeclaration
	Chats             map[string]*genai.Chat
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
		Chats:             make(map[string]*genai.Chat),
	}
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
	var err error
	chat := a.Chats[sessionID]
	if chat == nil {
		chat, err = a.client.Chats.Create(ctx, a.model, &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: a.systemInstruction}},
			},
			Tools: a.getTools(),
		}, []*genai.Content{})
		if err != nil {
			return nil, err
		}

		a.Chats[sessionID] = chat
	}

	return chat, nil
}

func (a *Agent) Send(ctx context.Context, sessionID string, prompt string) ([]Content, error) {
	chat, err := a.getChat(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	resp, err := chat.SendMessage(ctx, genai.Part{
		Text: prompt,
	})

	contents, err := a.processResponse(ctx, chat, resp)
	if err != nil {
		return nil, err
	}

	return a.parseContents(contents)
}

func (a *Agent) parseContents(contents []*genai.Content) ([]Content, error) {
	parsedContents := []Content{}
	for _, content := range contents {
		parsedParts, err := a.parseParts(content.Parts)
		if err != nil {
			return nil, err
		}

		parsedContents = append(parsedContents, Content{Parts: parsedParts, Role: content.Role})
	}

	return parsedContents, nil
}

func (a *Agent) parseParts(parts []*genai.Part) ([]Part, error) {
	parsedParts := []Part{}
	for _, part := range parts {
		if part.FunctionCall != nil {
			parsedParts = append(parsedParts, Part{FunctionCall: fmt.Sprintf("%s %v", part.FunctionCall.Name, part.FunctionCall.Args)})
			continue
		}

		if part.FunctionResponse != nil {
			parsedParts = append(parsedParts, Part{FunctionResponse: fmt.Sprintf("%s %v", part.FunctionResponse.Name, part.FunctionResponse.Response)})
			continue
		}

		parsedParts = append(parsedParts, Part{Text: part.Text})
	}

	return parsedParts, nil
}

func (a *Agent) handleFunctionCall(ctx context.Context, functionName string, args map[string]any) (map[string]any, error) {
	if fd, exists := a.functionsMap[functionName]; exists {
		return fd.FunctionCall(ctx, args)
	}

	return nil, fmt.Errorf("function %s not found", functionName)
}

func (a *Agent) ClearSession(sessionID string) {
	a.Chats[sessionID] = nil
}

func (a *Agent) GetSession(sessionID string) ([]Content, error) {
	chat := a.Chats[sessionID]
	if chat == nil {
		return []Content{}, nil
	}

	return a.parseContents(chat.History(true))
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
