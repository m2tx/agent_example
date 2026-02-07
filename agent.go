package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/genai"
)

type Part struct {
	Text string
}

type Content struct {
	Parts []Part
}

type Agent struct {
	client            *genai.Client
	model             string
	systemInstruction string
	functionsMap      map[string]FunctionCallFn
	tools             []*genai.Tool
	Session           map[string][]*genai.Content
}

type FunctionCallFn func(ctx context.Context, args map[string]any) (map[string]any, error)

func NewAgent(client *genai.Client, model string, systemInstruction string) *Agent {
	return &Agent{
		client:            client,
		model:             model,
		systemInstruction: systemInstruction,
		functionsMap:      make(map[string]FunctionCallFn),
		tools: []*genai.Tool{
			{
				FunctionDeclarations: []*genai.FunctionDeclaration{},
			},
		},
		Session: make(map[string][]*genai.Content),
	}
}

func (a *Agent) AddFunction(functionDeclaration *genai.FunctionDeclaration, toolFunc FunctionCallFn) error {
	a.functionsMap[functionDeclaration.Name] = toolFunc

	a.tools[0].FunctionDeclarations = append(a.tools[0].FunctionDeclarations, functionDeclaration)

	return nil
}

func (a *Agent) Send(ctx context.Context, sessionID string, prompt string) (*Content, error) {
	a.Session[sessionID] = append(a.Session[sessionID], &genai.Content{
		Parts: []*genai.Part{
			{
				Text: prompt,
			},
		},
		Role: genai.RoleUser,
	})

	resp, err := a.client.Models.GenerateContent(ctx, a.model, a.Session[sessionID], &genai.GenerateContentConfig{
		Tools: a.tools,
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
		parsedParts = append(parsedParts, Part{Text: part.Text})
	}

	return parsedParts, nil
}

func (a *Agent) HandleFunctionCall(ctx context.Context, functionName string, args map[string]any) (map[string]any, error) {
	if fn, exists := a.functionsMap[functionName]; exists {
		return fn(ctx, args)
	}

	return nil, fmt.Errorf("function %s not found", functionName)
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
