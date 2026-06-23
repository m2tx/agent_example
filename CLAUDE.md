# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> For guidance targeted at other AI coding agents (OpenAI Codex, GitHub Copilot, etc.), see [AGENTS.md](AGENTS.md).

## Project Overview

This is a Go-based agentic system that supports multiple LLM providers (Google Gemini and Anthropic Claude) to power a generic AI assistant with function calling and semantic document search capabilities. The agent can reason about user requests and invoke registered tools, maintaining multi-turn conversation sessions via a REST API.

## Build and Run Commands

```bash
# Install dependencies
go mod download
go mod vendor

# Run the server (default: port 8080, gemini-2.5-flash model, Gemini provider)
go run ./cmd/server

# Run with Anthropic provider
PROVIDER=anthropic ANTHROPIC_API_KEY=your-key go run ./cmd/server

# Run with custom configuration
HTTP_PORT=8081 MODEL=gemini-2.5-pro go run ./cmd/server

# Build a binary
go build -o agent ./cmd/server

# Format code
go fmt ./...

# Lint code (using golangci-lint, if available)
golangci-lint run ./...

# Vet code for common issues
go vet ./...
```

## High-Level Architecture

The project implements an **agentic system with automatic function calling**. Here's how it works:

### Core Flow

1. **Client Request**: User sends a prompt via `/prompt` endpoint with a `session_id`
2. **Agent Processing**: The agent passes the prompt to the configured LLM provider along with declared tool functions
3. **Function Invocation**: If the LLM requests a tool call, the agent automatically executes it via registered handlers
4. **Feedback Loop**: Function responses are sent back to the LLM for reasoning until completion
5. **Response**: Final response is returned and stored in session history

### Key Components

- **`agent.Agent`** (`internal/agent/agent.go`): Core agent orchestrator
  - Handles function declaration registration via `AddFunctionCall()`
  - Delegates LLM interaction to the configured `LLMProvider`
  - Manages session history: load → send → save
  - Returns parsed content with both text and function call information

- **`agent.LLMProvider`** (`internal/agent/provider.go`): Provider interface
  - `Send()` for blocking responses; `SendStream()` for streaming
  - Each implementation receives the full conversation history and returns only new content produced during the turn
  - Implementations: Gemini (`internal/provider/gemini/`) and Anthropic (`internal/provider/anthropic/`)

- **`gemini.Provider`** (`internal/provider/gemini/gemini.go`): Google Gemini backend
  - Uses `google.golang.org/genai` SDK
  - Handles recursive function call processing via `processResponse()`
  - Supports streaming via `processResponseStream()`

- **`anthropic.Provider`** (`internal/provider/anthropic/anthropic.go`): Anthropic Claude backend
  - Uses `github.com/anthropics/anthropic-sdk-go`
  - Implements the same tool-use loop with `StopReasonToolUse` detection
  - Supports streaming via `Messages.NewStreaming()`

- **`agent.Embedder`** (`internal/agent/embedder.go`): Document embedder for semantic search
  - Indexes a directory of documents at startup using Gemini embedding models
  - Used by the `search_docs` function to find relevant documentation

- **`FunctionDeclaration`**: Wrapper for tool metadata and implementation
  - Defines the tool's schema (name, description, parameters, response structure)
  - Contains the actual implementation function (`FunctionCall`)
  - Follows the JSON schema pattern that both Gemini and Anthropic understand

- **`mcp.Client`** (`internal/mcp/mcp.go`): MCP client for dynamic tool registration
  - Connects to an MCP server via HTTP streamable transport (`MCP_SERVER_URL`, default `http://localhost:9000`)
  - Transport type configurable via `MCP_TRANSPORT` env var
  - `NewClient(ctx, endpoint)` establishes the session; `RegisterTools(ctx, registry)` fetches and registers all server tools
  - MCP server is a **required** dependency — the server will fatal on startup if it cannot connect

- **Server** (`cmd/server/main.go`): REST API server
  - Routes: `/` (UI), `/prompt` (POST to chat), `/history` (GET/DELETE session)
  - Loads system instruction from embedded assets (`assets/system_instruction.md`)
  - Selects provider via `buildProvider()` based on `PROVIDER` env var
  - Initializes `Embedder` and indexes `docs/` directory at startup
  - Connects to MCP server and registers its tools and prompts, then registers built-in functions: `get_weather`, `get_companies`, `get_collaborators`, `search_docs`

### Function Declaration Pattern

Adding a new tool follows this pattern (see `internal/functions/` for examples):

1. Create a `CreateXFunctionDeclaration()` function that returns `*agent.FunctionDeclaration`
2. Define the function's JSON schema for parameters and response
3. Implement the `FunctionCall` handler (receives args as `map[string]any`)
4. Register it in `main()` via `agent.AddFunctionCall()`

The agent automatically handles:
- Converting tool calls from the LLM's format to invocation
- Passing function responses back to the LLM for continued reasoning
- Collecting the final response after all tool calls complete

### Session Management

- Each call to `Send()` loads history from MongoDB, delegates to the provider, then saves history back after the response
- History is retrieved via `GetSession(sessionID)` (reads from MongoDB)
- Cleared via `ClearSession(sessionID)` (deletes from MongoDB)
- The `Content` and `Part` types represent the serializable model stored in MongoDB
- Both providers translate between the internal `model.Content` format and their own SDK types

## Configuration

Environment variables:
- `PROVIDER`: LLM provider to use — `gemini` (default) or `anthropic`
- `GEMINI_API_KEY`: Required when `PROVIDER=gemini`. Google API key for Gemini access
- `ANTHROPIC_API_KEY`: Required when `PROVIDER=anthropic`. Anthropic API key
- `MODEL`: Model name to use. Defaults: `gemini-2.5-flash` (Gemini), `claude-opus-4-7` (Anthropic)
- `HTTP_PORT`: Server port (default: `8080`)
- `MONGODB_URI`: MongoDB connection URI (default: `mongodb://localhost:27017`)
- `MONGODB_DB`: MongoDB database name (default: `agent_sessions`)
- `MCP_SERVER_URL`: MCP server URL using HTTP streamable transport (default: `http://localhost:9000`). Required — server will not start if unreachable.
- `MCP_TRANSPORT`: MCP transport type (default: streamable HTTP)

## Important Implementation Details

- **Provider Selection**: `buildProvider()` in `main.go` reads `PROVIDER` env var and constructs the appropriate `LLMProvider` implementation. Adding a new provider means implementing the `LLMProvider` interface and adding a case there.
- **Session Storage**: MongoDB-backed via `repository.SessionRepository`. Each `Send()` call loads history and saves updated history back. No in-memory chat cache.
- **System Instruction**: Loaded from `assets/system_instruction.md` and embedded in binary. Configured as a generic AI assistant that uses `search_docs` before answering when documentation may be relevant.
- **Document Indexing**: `Embedder` indexes the `docs/` directory at startup; the `search_docs` tool uses these embeddings for semantic retrieval. Embedder always uses Gemini regardless of `PROVIDER`.
- **Tool-use Loop**: Both providers implement an internal loop that continues sending tool results back to the LLM until the response contains no more function calls.
- **Error Handling**: LLM API errors propagate to HTTP client; function execution errors are returned to the LLM
- **No Tests**: Currently no test coverage. Suggested areas: function handlers, provider translation logic, session management
