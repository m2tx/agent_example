package functions

import (
	"context"

	"github.com/m2tx/agent_example/internal/agent"
)

func CreateCompanyFunctionDeclaration() *agent.FunctionDeclaration {
	return &agent.FunctionDeclaration{
		Name:        "get_companies",
		Description: "Busca as empresas que tenho acesso.",
		ResponseSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"companies": map[string]any{
					"type":        "array",
					"description": "A lista de empresas",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{
								"type":        "string",
								"description": "O ID da empresa",
							},
							"name": map[string]any{
								"type":        "string",
								"description": "O nome da empresa",
							},
						},
					},
				},
			},
		},
		FunctionCall: func(ctx context.Context, args map[string]any) (map[string]any, error) {
			return map[string]any{
				"companies": []map[string]any{
					{
						"id":   "1",
						"name": "Empresa A",
					},
					{
						"id":   "2",
						"name": "Empresa B",
					},
				},
			}, nil
		},
	}
}

func CreateCollaboratorsFunctionDeclaration() *agent.FunctionDeclaration {
	return &agent.FunctionDeclaration{
		Name:        "get_collaborators",
		Description: "Busca os colaboradores da empresa.",
		ParametersSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"company_id": map[string]any{
					"type":        "string",
					"description": "O ID da empresa",
				},
			},
			"required": []string{"company_id"},
		},
		ResponseSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collaborators": map[string]any{
					"type":        "array",
					"description": "A lista de colaboradores da empresa",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{
								"type":        "string",
								"description": "O ID do colaborador",
							},
							"name": map[string]any{
								"type":        "string",
								"description": "O nome do colaborador",
							},
						},
					},
				},
			},
		},
		FunctionCall: func(ctx context.Context, args map[string]any) (map[string]any, error) {
			argsCompanyID, _ := args["company_id"].(string)
			if argsCompanyID == "1" {
				return map[string]any{
					"collaborators": []map[string]any{
						{
							"id":   "1",
							"name": "Fulano da Silva",
						},
						{
							"id":   "2",
							"name": "Beltrano da Silva",
						},
					},
				}, nil
			}

			if argsCompanyID == "2" {
				return map[string]any{
					"collaborators": []map[string]any{
						{
							"id":   "3",
							"name": "Ciclano da Silva",
						},
						{
							"id":   "4",
							"name": "Fulana da Costa",
						},
						{
							"id":   "5",
							"name": "Beltrana da Silva",
						},
					},
				}, nil
			}

			return map[string]any{
				"collaborators": []map[string]any{},
			}, nil
		},
	}
}
