# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based agentic system that uses Google's Gemini API to power an intelligent agent with function calling capabilities. The agent can reason about user requests and invoke registered tools to retrieve information, maintaining multi-turn conversation sessions via a REST API.

## Build and Run Commands

```bash
# Install dependencies
go mod download
go mod vendor

# Run the server (default: port 8080, gemini-2.5-flash model)
go run ./cmd/server

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
2. **Agent Processing**: The agent passes the prompt to Gemini API along with declared tool functions
3. **Function Invocation**: If Gemini requests a tool call, the agent automatically executes it via registered handlers
4. **Feedback Loop**: Function responses are sent back to Gemini for reasoning until completion
5. **Response**: Final response is returned and stored in session history

### Key Components

- **`agent.Agent`** (`internal/agent/agent.go`): Core agent orchestrator
  - Manages Gemini chat sessions per session ID (stored in `Chats` map)
  - Handles function declaration registration via `AddFunctionCall()`
  - Processes Gemini responses and automatically invokes tool functions via `handleFunctionCall()`
  - Recursively processes responses until no more function calls are needed (`processResponse()`)
  - Returns parsed content with both text and function call information

- **`FunctionDeclaration`**: Wrapper for tool metadata and implementation
  - Defines the tool's schema (name, description, parameters, response structure)
  - Contains the actual implementation function (`FunctionCall`)
  - Follows the JSON schema pattern that Gemini understands

- **Server** (`cmd/server/main.go`): REST API server
  - Routes: `/` (UI), `/prompt` (POST to chat), `/history` (GET/DELETE session)
  - Loads system instruction from embedded assets
  - Initializes agent with registered functions

### Function Declaration Pattern

Adding a new tool follows this pattern (see `internal/functions/` for examples):

1. Create a `CreateXFunctionDeclaration()` function that returns `*agent.FunctionDeclaration`
2. Define the function's JSON schema for parameters and response
3. Implement the `FunctionCall` handler (receives args as `map[string]any`)
4. Register it in `main()` via `agent.AddFunctionCall()`

The agent automatically handles:
- Converting tool calls from Gemini's format to invocation
- Passing function responses back to Gemini for continued reasoning
- Collecting the final response after all tool calls complete

### Session Management

- Each call to `Send()` loads history from MongoDB, creates a fresh `genai.Chat`, then saves history back after the response
- History is retrieved via `GetSession(sessionID)` (reads from MongoDB)
- Cleared via `ClearSession(sessionID)` (deletes from MongoDB)
- The `Content` and `Part` types represent the serializable model stored in MongoDB

## Configuration

Environment variables:
- `GEMINI_API_KEY`: Required. Google API key for Gemini access
- `MODEL`: Gemini model to use (default: `gemini-2.5-flash`). Try `gemini-2.5-pro` for better reasoning
- `HTTP_PORT`: Server port (default: `8080`)
- `MONGODB_URI`: MongoDB connection URI (default: `mongodb://localhost:27017`)
- `MONGODB_DB`: MongoDB database name (default: `agent_sessions`)

## Important Implementation Details

- **Session Storage**: MongoDB-backed via `repository.SessionRepository`. Each `Send()` call loads history, creates a fresh `genai.Chat`, then saves updated history back. No in-memory chat cache.
- **System Instruction**: Loaded from `assets/system_instruction.txt` and embedded in binary
- **Recursive Processing**: `processResponse()` is recursive to handle chains of function calls
- **Error Handling**: Gemini API errors propagate to HTTP client; function execution errors are returned to Gemini
- **No Tests**: Currently no test coverage. Suggested areas: function handlers, agent response parsing, session management
