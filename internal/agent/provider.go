package agent

import (
	"context"

	"github.com/m2tx/agent_example/internal/model"
)

// ProviderRequest is a provider-agnostic bundle for a single chat turn.
type ProviderRequest struct {
	SystemInstruction  string
	History            []model.Content
	Tools              map[string]*FunctionDeclaration
	HandleFunctionCall func(ctx context.Context, name string, args map[string]any) (map[string]any, error)
	Prompt             string
}

// LLMProvider abstracts a backend LLM (Gemini, Anthropic, etc.).
// Implementations receive the full conversation history and return only the
// new content produced during this turn (user message + model response(s) +
// any tool results), ready to be appended and persisted by the agent.
type LLMProvider interface {
	Send(ctx context.Context, req ProviderRequest) ([]model.Content, error)
	SendStream(ctx context.Context, req ProviderRequest, onText func(string) error, onFunctionCall func(name string, args map[string]any) error, onTurnDone func() error) ([]model.Content, error)
}
