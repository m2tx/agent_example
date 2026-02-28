package functions

import (
	"context"
	"fmt"

	"github.com/m2tx/agent_example/internal/agent"
)

func CreateWeatherFunctionDeclaration() *agent.FunctionDeclaration {
	return &agent.FunctionDeclaration{
		Name:        "get_weather",
		Description: "Busca o clima atual de uma cidade",
		ParametersSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{
					"type":        "string",
					"description": "A cidade, ex: São Paulo, SP",
				},
			},
			"required": []string{"location"},
		},
		ResponseSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{
					"type":        "string",
					"description": "A cidade, ex: São Paulo, SP",
				},
				"temperature": map[string]any{
					"type":        "string",
					"description": "Temperatura atual, ex: 22°C",
				},
				"condition": map[string]any{
					"type":        "string",
					"description": "Condição do tempo, ex: Ensolarado",
				},
			},
		},
		FunctionCall: func(ctx context.Context, args map[string]any) (map[string]any, error) {
			location, ok := args["location"].(string)
			if !ok {
				return nil, fmt.Errorf("invalid location argument")
			}

			return map[string]any{
				"location":    location,
				"temperature": "22°C",
				"condition":   "Ensolarado",
			}, nil
		},
	}
}
