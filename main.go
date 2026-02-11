package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/m2tx/agent_example/agent"
	"google.golang.org/genai"
)

//go:embed system_instruction.txt
var systemInstruction string

func main() {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal(err)
	}

	a := agent.New(client, getModel(), systemInstruction)
	err = a.AddFunctionCall(createWeatherFunctionDeclaration())
	if err != nil {
		log.Fatal(err)
	}

	err = a.AddFunctionCall(createCompanyFunctionDeclaration())
	if err != nil {
		log.Fatal(err)
	}

	err = a.AddFunctionCall(createCollaboratorsFunctionDeclaration())
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			http.Error(w, "session_id is required", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")

		contents := a.GetSession(sessionID)
		json.NewEncoder(w).Encode(contents)
	})

	http.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SessionID string `json:"session_id"`
			Prompt    string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.SessionID == "" {
			http.Error(w, "session_id is required", http.StatusBadRequest)
			return
		}

		if req.Prompt == "" {
			http.Error(w, "prompt is required", http.StatusBadRequest)
			return
		}

		resp, err := a.Send(ctx, req.SessionID, []string{"get_companies", "get_collaborators"}, req.Prompt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")

		json.NewEncoder(w).Encode(resp)
	})

	log.Fatal(http.ListenAndServe(":"+getHttpPort(), nil))
}

func getModel() string {
	model := os.Getenv("MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
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
