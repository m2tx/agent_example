# Agent Example

A Go-based agentic system powered by Google's Gemini API with function calling, MongoDB-backed session persistence, and a REST API with a built-in chat UI.

## Features

- **Intelligent Agent**: Powered by Gemini API with configurable system instructions
- **Function Calling**: The agent can invoke tools:
  - `get_weather` - Retrieve current weather information for a location
  - `get_companies` - List accessible companies
  - `get_collaborators` - Retrieve employee/collaborator information for a company
- **Session Persistence**: Conversation history stored in MongoDB per session
- **REST API**:
  - `GET /` - Built-in chat UI
  - `POST /prompt` - Send a prompt and get a response
  - `GET /history?session_id=<id>` - Retrieve session conversation history
  - `DELETE /history?session_id=<id>` - Clear a session
- **Configurable**: Environment variables for model selection, HTTP port, and MongoDB connection

## Architecture

```
cmd/server/main.go              # HTTP server, agent setup, route handlers
internal/agent/agent.go         # Core agent: session management, function call loop
internal/functions/
  weather.go                    # Weather function declaration
  company.go                    # Company and collaborator function declarations
internal/model/content.go       # Content/Part types for serializable history
internal/repository/
  repository.go                 # SessionRepository interface
  mongodb.go                    # MongoDB-backed session persistence
assets/
  chat.html                     # Embedded chat UI
  system_instruction.txt        # Embedded system prompt
```

### How It Works

1. The client sends a prompt to `/prompt` with a `session_id`
2. The agent loads session history from MongoDB and passes the prompt to Gemini
3. If Gemini requests a tool call, the agent executes it and feeds the result back
4. This loop continues until Gemini returns a final text response
5. The updated history is saved back to MongoDB

### Adding a New Tool

1. Create a `CreateXFunctionDeclaration()` in `internal/functions/` returning `*agent.FunctionDeclaration`
2. Define the JSON schema for parameters and response
3. Implement the `FunctionCall` handler (`map[string]any` → `any, error`)
4. Register it in `main()` via `a.AddFunctionCall()`

## Getting Started

### Prerequisites

- Go 1.21+
- MongoDB instance
- Gemini API key — set `GEMINI_API_KEY` environment variable

### Installation

```bash
go mod download
go mod vendor
```

### Running

```bash
# Default: port 8080, gemini-2.5-flash, MongoDB at localhost:27017
go run ./cmd/server

# Custom configuration
HTTP_PORT=8081 MODEL=gemini-2.5-pro MONGODB_URI=mongodb://host:27017 go run ./cmd/server

# Build a binary
go build -o agent ./cmd/server
```

## Usage

### Send a Prompt

```bash
curl -X POST http://localhost:8080/prompt \
  -H "Content-Type: application/json" \
  -d '{"session_id": "user-123", "prompt": "What companies do I have access to?"}'
```

### Retrieve Session History

```bash
curl http://localhost:8080/history?session_id=user-123
```

### Clear a Session

```bash
curl -X DELETE http://localhost:8080/history?session_id=user-123
```

## Configuration

| Variable       | Default                     | Description                        |
|----------------|-----------------------------|------------------------------------|
| `GEMINI_API_KEY` | *(required)*              | Google API key for Gemini access   |
| `MODEL`        | `gemini-2.5-flash`          | Gemini model to use                |
| `HTTP_PORT`    | `8080`                      | HTTP server port                   |
| `MONGODB_URI`  | `mongodb://localhost:27017` | MongoDB connection URI             |
| `MONGODB_DB`   | `agent_sessions`            | MongoDB database name              |
