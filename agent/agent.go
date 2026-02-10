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
	Session           map[string][]*genai.Content
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
		Session:           make(map[string][]*genai.Content),
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
	a.Session[sessionID] = append(a.Session[sessionID], &genai.Content{
		Parts: []*genai.Part{
			{
				Text: prompt,
			},
		},
		Role: genai.RoleUser,
	})

	resp, err := a.client.Models.GenerateContent(ctx, a.model, a.Session[sessionID], &genai.GenerateContentConfig{
		Tools: a.getTools(tools),
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: a.systemInstruction}},
		},
	})
	if err != nil {
		return nil, err
	}

	partsGen, err := a.processResponse(ctx, sessionID, resp)
	if err != nil {
		return nil, err
	}

	parts, err := a.parseParts(partsGen)
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

func (a *Agent) HandleFunctionCall(ctx context.Context, functionName string, args map[string]any) (map[string]any, error) {
	if fd, exists := a.functionsMap[functionName]; exists {
		return fd.FunctionCall(ctx, args)
	}

	return nil, fmt.Errorf("function %s not found", functionName)
}

func (a *Agent) ClearSession(sessionID string) {
	a.Session[sessionID] = []*genai.Content{}
}

func (a *Agent) GetSession(sessionID string) []Content {
	contentList := a.Session[sessionID]
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

func (a *Agent) processResponse(ctx context.Context, sessionID string, resp *genai.GenerateContentResponse) ([]*genai.Part, error) {
	parts := []*genai.Part{}
	for _, candidate := range resp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}

		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				log.Printf("FunctionCall: %s %v\n", part.FunctionCall.Name, part.FunctionCall.Args)
				funcResp, err := a.HandleFunctionCall(ctx, part.FunctionCall.Name, part.FunctionCall.Args)
				if err != nil {
					return nil, err
				}

				a.Session[sessionID] = append(a.Session[sessionID], candidate.Content, &genai.Content{
					Parts: []*genai.Part{
						{
							FunctionResponse: &genai.FunctionResponse{
								Name:     part.FunctionCall.Name,
								Response: funcResp,
							},
						},
					},
				})

				resp, err := a.client.Models.GenerateContent(ctx, a.model, a.Session[sessionID], &genai.GenerateContentConfig{})
				if err != nil {
					return nil, err
				}

				return a.processResponse(ctx, sessionID, resp)
			}
		}

		a.Session[sessionID] = append(a.Session[sessionID], candidate.Content)

		parts = append(parts, candidate.Content.Parts...)
	}

	return parts, nil
}
