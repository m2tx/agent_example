package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal(err)
	}

	agent := NewAgent(client, getModel(), "Você é um agente de suporte que usa ferramentas para responder.")
	err = agent.AddFunction(&genai.FunctionDeclaration{
		Name:        "get_weather",
		Description: "Busca o clima atual de uma cidade",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"location": {
					Type:        genai.TypeString,
					Description: "A cidade, ex: São Paulo, SP",
				},
			},
			Required: []string{"location"},
		},
	}, func(ctx context.Context, args map[string]any) (map[string]any, error) {
		location, ok := args["location"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid location argument")
		}

		return map[string]any{
			"location":    location,
			"temperature": "22°C",
			"condition":   "Ensolarado",
		}, nil
	})
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SessionID string `json:"session_id"`
			Prompt    string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := agent.Send(ctx, req.SessionID, req.Prompt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(resp)
	})

	log.Fatal(http.ListenAndServe(":"+getHttpPort(), nil))
}

func getModel() string {
	model := os.Getenv("MODEL")
	if model == "" {
		model = "gemini-2.5-pro"
	}

	return model
}

func getHttpPort() string {
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	return port
}
