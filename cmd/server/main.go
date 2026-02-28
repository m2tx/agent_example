package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/m2tx/agent_example/assets"
	"github.com/m2tx/agent_example/internal/agent"
	"github.com/m2tx/agent_example/internal/functions"
	"github.com/m2tx/agent_example/internal/repository"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(getMongoURI()))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := mongoClient.Disconnect(ctx); err != nil {
			log.Printf("mongodb disconnect: %v", err)
		}
	}()

	database := mongoClient.Database(getMongoDB())

	repo := repository.NewMongoSessionRepository(database, "sessions")

	a := agent.NewWithRepo(client, getModel(), assets.SystemInstruction, repo)
	err = a.AddFunctionCall(functions.CreateWeatherFunctionDeclaration())
	if err != nil {
		log.Fatal(err)
	}

	err = a.AddFunctionCall(functions.CreateCompanyFunctionDeclaration())
	if err != nil {
		log.Fatal(err)
	}

	err = a.AddFunctionCall(functions.CreateCollaboratorsFunctionDeclaration())
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, assets.Dir, "chat.html")
	})

	http.HandleFunc("/history", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodDelete {
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

		if r.Method == http.MethodGet {
			contents, err := a.GetSession(r.Context(), sessionID)
			if err != nil {
				http.Error(w, "get session", http.StatusInternalServerError)
				return
			}

			json.NewEncoder(w).Encode(contents)
		}

		if r.Method == http.MethodDelete {
			a.ClearSession(r.Context(), sessionID)
		}
	})

	http.HandleFunc("/prompt", func(w http.ResponseWriter, r *http.Request) {
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

		resp, err := a.Send(r.Context(), req.SessionID, req.Prompt)
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

func getMongoURI() string {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	return uri
}

func getMongoDB() string {
	db := os.Getenv("MONGODB_DB")
	if db == "" {
		db = "agent_sessions"
	}

	return db
}
