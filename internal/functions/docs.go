package functions

import (
	"context"
	"fmt"

	"github.com/m2tx/agent_example/internal/agent"
)

// CreateDocsSearchFunctionDeclaration returns an agent tool that semantically
// searches documents indexed from the docs/ folder.
func CreateDocsSearchFunctionDeclaration(e *agent.Embedder) *agent.FunctionDeclaration {
	return &agent.FunctionDeclaration{
		Name:        "search_docs",
		Description: "Searches the internal document library for information relevant to the query. Use this whenever the user asks about topics that might be covered in internal documentation.",
		ParametersSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query describing what information you need",
				},
			},
			"required": []string{"query"},
		},
		ResponseSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"results": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"filename": map[string]any{
								"type":        "string",
								"description": "Source document filename",
							},
							"content": map[string]any{
								"type":        "string",
								"description": "Relevant text excerpt from the document",
							},
						},
					},
				},
			},
		},
		FunctionCall: func(ctx context.Context, args map[string]any) (map[string]any, error) {
			query, ok := args["query"].(string)
			if !ok || query == "" {
				return nil, fmt.Errorf("search_docs: query argument is required")
			}

			docs, err := e.Search(query, 3)
			if err != nil {
				return nil, fmt.Errorf("search_docs: %w", err)
			}

			results := make([]map[string]any, 0, len(docs))
			for _, doc := range docs {
				results = append(results, map[string]any{
					"filename": doc.Filename,
					"content":  doc.Text,
				})
			}

			return map[string]any{"results": results}, nil
		},
	}
}
