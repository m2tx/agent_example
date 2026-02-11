package agent

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/genai"
)

type Part struct {
	Text             string `json:"text,omitempty"`
	FunctionCall     string `json:"function_call,omitempty"`
	FunctionResponse string `json:"function_response,omitempty"`
}

type Content struct {
	Parts []Part `json:"parts"`
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

func (a *Agent) getTools(functionNames []string) []*genai.Tool {
	functions := []*genai.FunctionDeclaration{}

	for _, name := range functionNames {
		if fd, exists := a.functionsMap[name]; exists {
			functions = append(functions, &genai.FunctionDeclaration{
				Name:                 fd.Name,
				Description:          fd.Description,
				ParametersJsonSchema: fd.ParametersSchema,
				ResponseJsonSchema:   fd.ResponseSchema,
			})
		}
	}

	return []*genai.Tool{
		{
			FunctionDeclarations: functions,
		},
	}
}

func (a *Agent) Send(ctx context.Context, sessionID string, tools []string, prompt string) (*Content, error) {
	var chat *genai.Chat
	var err error

	chat = a.Chats[sessionID]
	if chat == nil {
		chat, err = a.client.Chats.Create(ctx, a.model, &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: a.systemInstruction}},
			},
			Tools: a.getTools(tools),
		}, []*genai.Content{})
		if err != nil {
			return nil, err
		}

		a.Chats[sessionID] = chat
	}

	resp, err := chat.SendMessage(ctx, genai.Part{
		Text: prompt,
	})

	resp, err = a.processResponse(ctx, chat, resp)
	if err != nil {
		return nil, err
	}

	parts, err := a.parseParts(resp.Candidates[0].Content.Parts)
	if err != nil {
		return nil, err
	}

	return &Content{Parts: parts}, nil
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

func (a *Agent) GetSession(sessionID string) []Content {
	chat := a.Chats[sessionID]
	if chat == nil {
		return []Content{}
	}

	contentList := chat.History(true)

	result := make([]Content, 0, len(contentList))
	for _, content := range contentList {
		parts, err := a.parseParts(content.Parts)
		if err != nil {
			log.Printf("Error parsing parts: %v", err)
			continue
		}

		result = append(result, Content{Parts: parts})
	}

	return result
}

func (a *Agent) processResponse(ctx context.Context, chat *genai.Chat, resp *genai.GenerateContentResponse) (*genai.GenerateContentResponse, error) {
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
	}

	if len(functionResponses) > 0 {
		resp, err := chat.SendMessage(ctx, functionResponses...)
		if err != nil {
			return nil, err
		}

		return a.processResponse(ctx, chat, resp)
	}

	return resp, nil
}
