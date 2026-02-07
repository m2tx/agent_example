# Agent Example

This project demonstrates how to build an intelligent agent with function calling capabilities, session management, and REST API integration.

## Overview

This project implements an agentic system that:
- Uses Google's Gemini 2.5 Pro model for intelligent reasoning
- Provides function calling capabilities for retrieving information
- Maintains conversation sessions for continuous multi-turn interactions
- Exposes a REST API for chat-based interactions
- Demonstrates real-world patterns for agent implementation in Go

## Features

- **Intelligent Agent**: Powered by Gemini API with configurable system instructions
- **Function Calling**: The agent can invoke tools:
  - `get_weather` - Retrieve current weather information for a location
  - `get_companies` - List accessible companies
  - `get_collaborators` - Retrieve employee/collaborator information for a company
- **Session Management**: Maintains conversation history per session
- **REST API**: 
  - `GET /session?session_id=<id>` - Retrieve session conversation history
  - `POST /chat` - Send messages and interact with the agent
- **Configurable**: Environment variables for model selection and HTTP port

## Architecture

```
main.go                 # HTTP server and agent setup
agent/agent.go          # Core agent implementation with session management
company.go              # Company and collaborator function declarations
weather.go              # Weather function declaration
```

The agent implements a function declaration pattern that allows you to:
1. Define function metadata (name, description, parameter schema, response schema)
2. Provide implementation functions that the agent can call
3. Automatically handle function calls and responses in the agent's reasoning loop

## Getting Started

### Prerequisites

- Go 1.20+
- Google Cloud credentials (for Gemini API access)
- Set `GEMINI_API_KEY` environment variable

### Installation

```bash
go mod download
go mod vendor
```

### Running

```bash
# Default: runs on port 8080 with gemini-2.5-pro model
go run *.go

# With custom configuration
HTTP_PORT=8081 MODEL=gemini-2.5-flash go run *.go
```

## Usage

### Start a Conversation

```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "user-123",
    "prompt": "Quais são as empresas que tenho acesso?"
  }'
```

### Retrieve Session History

```bash
curl http://localhost:8080/session?session_id=user-123
```

## Configuration

- `HTTP_PORT`: HTTP server port (default: 8080)
- `MODEL`: Gemini model to use (default: gemini-2.5-pro)
- `GEMINI_API_KEY`: Google API key for Gemini access (required)

## Notes

- Session data is stored in-memory; consider persistent storage for production
- Function implementations can be extended to integrate with actual backend systems and data sources
