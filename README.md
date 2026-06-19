# Agent Example

A Go-based agentic system with multi-provider LLM support (Google Gemini and Anthropic Claude), function calling, MongoDB-backed session persistence, semantic document search, and a REST API with a built-in chat UI.

## Features

- **Multi-Provider Support**: Switch between Google Gemini and Anthropic Claude via the `PROVIDER` environment variable
- **Function Calling**: The agent can invoke tools:
  - `get_weather` - Retrieve current weather information for a location
  - `get_companies` - List accessible companies
  - `get_collaborators` - Retrieve employee/collaborator information for a company
  - `search_docs` - Semantic search over indexed documentation using embeddings
- **MCP Integration**: Dynamically registers tools and prompts from an external MCP server via HTTP (streamable transport)
- **Document Indexing**: Automatically indexes a docs directory at startup using Gemini embeddings
- **Session Persistence**: Conversation history stored in MongoDB per session
- **Streaming**: Server-Sent Events (`text/event-stream`) for real-time response delivery
- **REST API**:
  - `GET /` - Built-in chat UI
  - `POST /prompt` - Send a prompt and stream the response
  - `GET /history?session_id=<id>` - Retrieve session conversation history
  - `DELETE /history?session_id=<id>` - Clear a session
- **Configurable**: Environment variables for provider selection, model, HTTP port, MongoDB connection, and MCP server URL

## Architecture

```
cmd/server/main.go              # HTTP server, provider selection, agent setup, route handlers
internal/agent/
  agent.go                      # Core agent: session management, function dispatch
  provider.go                   # LLMProvider interface
  embedder.go                   # Gemini-based document embedder for semantic search
internal/provider/
  gemini/gemini.go              # Google Gemini provider implementation
  anthropic/anthropic.go        # Anthropic Claude provider implementation
internal/functions/
  weather.go                    # Weather function declaration
  company.go                    # Company and collaborator function declarations
  docs.go                       # Docs semantic search function declaration
internal/mcp/
  mcp.go                        # MCP client: connects to MCP server and registers tools/prompts
internal/model/content.go       # Content/Part types for serializable history
internal/repository/
  repository.go                 # SessionRepository interface
  mongodb.go                    # MongoDB-backed session persistence
assets/
  chat.html                     # Embedded chat UI
  system_instruction.md         # Embedded system prompt (generic AI assistant)
docs/                           # Documents indexed at startup for search_docs tool
```

### How It Works

1. The client sends a prompt to `/prompt` with a `session_id`
2. The agent loads session history from MongoDB and passes the prompt to the configured LLM provider
3. If the LLM requests a tool call, the agent executes it and feeds the result back
4. This loop continues until the LLM returns a final text response
5. The updated history is saved back to MongoDB

### LLM Provider Interface

Both Gemini and Anthropic implement the same `LLMProvider` interface:

```go
type LLMProvider interface {
    Send(ctx context.Context, req ProviderRequest) ([]model.Content, error)
    SendStream(ctx context.Context, req ProviderRequest, onText func(string) error, onFunctionCall func(name string, args map[string]any) error) ([]model.Content, error)
}
```

Each provider translates between the shared `model.Content` history format and its own SDK types, handling the tool-use loop internally.

### Adding a New Tool

1. Create a `CreateXFunctionDeclaration()` in `internal/functions/` returning `*agent.FunctionDeclaration`
2. Define the JSON schema for parameters and response
3. Implement the `FunctionCall` handler (`map[string]any` → `map[string]any, error`)
4. Register it in `main()` via `a.AddFunctionCall()`

## Getting Started

### Prerequisites

- Go 1.21+
- MongoDB instance
- API key for your chosen provider:
  - **Gemini**: set `GEMINI_API_KEY`
  - **Anthropic**: set `ANTHROPIC_API_KEY`
- MCP server running and accessible (default: `http://localhost:9000`)

### Installation

```bash
go mod download
go mod vendor
```

### Running

```bash
# Default: Gemini provider, gemini-2.5-flash, port 8080, MongoDB at localhost:27017
GEMINI_API_KEY=your-api-key go run ./cmd/server

# Anthropic provider with Claude
PROVIDER=anthropic ANTHROPIC_API_KEY=your-api-key go run ./cmd/server

# Custom Gemini configuration
GEMINI_API_KEY=your-api-key HTTP_PORT=8081 MODEL=gemini-2.5-pro MONGODB_URI=mongodb://host:27017 MCP_SERVER_URL=http://mcp-host:9000 go run ./cmd/server

# Build a binary
go build -o agent ./cmd/server
./agent
```

## Usage

### Send a Prompt

```bash
curl -X POST http://localhost:8080/prompt \
  -H "Content-Type: application/json" \
  -d '{"session_id": "user-123", "prompt": "What is the weather in London?"}'
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

| Variable            | Default                     | Description                                              |
|---------------------|-----------------------------|----------------------------------------------------------|
| `PROVIDER`          | `gemini`                    | LLM provider: `gemini` or `anthropic`                   |
| `GEMINI_API_KEY`    | *(required for Gemini)*     | Google API key for Gemini access                         |
| `ANTHROPIC_API_KEY` | *(required for Anthropic)*  | Anthropic API key                                        |
| `MODEL`             | provider-dependent          | Model name (`gemini-2.5-flash` or `claude-opus-4-7`)    |
| `HTTP_PORT`         | `8080`                      | HTTP server port                                         |
| `MONGODB_URI`       | `mongodb://localhost:27017` | MongoDB connection URI                                   |
| `MONGODB_DB`        | `agent_sessions`            | MongoDB database name                                    |
| `MCP_SERVER_URL`    | `http://localhost:9000`     | MCP server URL (HTTP streamable transport)               |
| `MCP_TRANSPORT`     | *(streamable HTTP)*         | MCP transport type                                       |
