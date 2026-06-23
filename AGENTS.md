# AGENTS.md

Guidance for AI coding agents working in this repository.

## Build & Run

```bash
# Install dependencies
go mod download
go mod vendor

# Run (default: Gemini provider, port 8080, MongoDB at localhost:27017)
GEMINI_API_KEY=your-key go run ./cmd/server

# Run with Anthropic
PROVIDER=anthropic ANTHROPIC_API_KEY=your-key go run ./cmd/server

# Build binary
go build -o agent ./cmd/server

# Format / vet
go fmt ./...
go vet ./...
```

## Environment Variables

| Variable            | Default                     | Description                                   |
|---------------------|-----------------------------|-----------------------------------------------|
| `PROVIDER`          | `gemini`                    | `gemini` or `anthropic`                       |
| `GEMINI_API_KEY`    | *(required for Gemini)*     | Google API key                                |
| `ANTHROPIC_API_KEY` | *(required for Anthropic)*  | Anthropic API key                             |
| `MODEL`             | `gemini-2.5-flash` / `claude-opus-4-7` | Model name (provider-dependent)  |
| `HTTP_PORT`         | `8080`                      | HTTP server port                              |
| `MONGODB_URI`       | `mongodb://localhost:27017` | MongoDB connection URI                        |
| `MONGODB_DB`        | `agent_sessions`            | MongoDB database name                         |
| `MCP_SERVER_URL`    | `http://localhost:9000`     | MCP server URL (required at startup)          |
| `MCP_TRANSPORT`     | *(streamable HTTP)*         | MCP transport type                            |

## Architecture

```
cmd/server/main.go              # HTTP server, provider selection, agent wiring
internal/agent/
  agent.go                      # Agent: session load → provider send → session save
  provider.go                   # LLMProvider interface (Send / SendStream)
  embedder.go                   # Gemini-based document embedder for search_docs
internal/provider/
  gemini/gemini.go              # Gemini backend (tool-use loop via processResponse)
  anthropic/anthropic.go        # Anthropic backend (StopReasonToolUse loop)
internal/functions/
  weather.go                    # get_weather tool
  company.go                    # get_companies / get_collaborators tools
  docs.go                       # search_docs tool (uses Embedder)
internal/mcp/
  mcp.go                        # MCP client: connects and registers tools/prompts
internal/model/content.go       # Content/Part types for serializable history
internal/repository/
  repository.go                 # SessionRepository interface
  mongodb.go                    # MongoDB implementation
assets/
  chat.html                     # Embedded chat UI
  system_instruction.md         # Embedded system prompt
docs/                           # Documents indexed at startup for search_docs
```

### Request Flow

1. `POST /prompt` → `agent.SendStream()` loads session history from MongoDB
2. History + prompt + tools sent to the configured `LLMProvider`
3. If the LLM returns a tool call, the agent executes it and feeds the result back
4. Loop continues until the LLM returns a final text response
5. Updated history saved back to MongoDB; SSE stream closed

### LLMProvider Interface

```go
type LLMProvider interface {
    Send(ctx context.Context, req ProviderRequest) ([]model.Content, error)
    SendStream(ctx context.Context, req ProviderRequest,
        onText func(string) error,
        onFunctionCall func(name string, args map[string]any) error,
        onTurnDone func() error,
    ) ([]model.Content, error)
}
```

Each provider receives the full conversation history and returns only the new content produced during the turn (user message + model response(s) + any tool results).

## Adding a New Tool

1. Create `CreateXFunctionDeclaration()` in `internal/functions/` returning `*agent.FunctionDeclaration`
2. Define the JSON schema for parameters and response (`ParametersSchema`, `ResponseSchema`)
3. Implement `FunctionCall func(ctx context.Context, args map[string]any) (map[string]any, error)`
4. Register it in `main()` via `a.AddFunctionCall(functions.CreateXFunctionDeclaration())`

## Adding a New LLM Provider

1. Implement the `agent.LLMProvider` interface
2. Add a case in `buildProvider()` in `cmd/server/main.go`

## Testing

No automated tests exist. Manual testing:

```bash
# Send a prompt
curl -X POST http://localhost:8080/prompt \
  -H "Content-Type: application/json" \
  -d '{"session_id": "test", "prompt": "Hello"}'

# Get session history
curl http://localhost:8080/history?session_id=test

# Clear session
curl -X DELETE http://localhost:8080/history?session_id=test
```

## Notes

- The MCP server (`MCP_SERVER_URL`) is a **required** dependency — the server fatals on startup if unreachable
- The `Embedder` always uses Gemini regardless of the `PROVIDER` setting
- Session history is stored in MongoDB; there is no in-memory cache
- `docs/` directory is indexed at startup; add `.md` files there to make them searchable via `search_docs`
