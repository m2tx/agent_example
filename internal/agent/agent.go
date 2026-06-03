package agent

import (
	"context"
	"fmt"

	"github.com/m2tx/agent_example/internal/model"
	"github.com/m2tx/agent_example/internal/repository"
)

type contextKey int

const sessionIDKey contextKey = iota

// WithSessionID returns a context carrying the given session ID.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

// SessionIDFromContext extracts the session ID stored by WithSessionID.
func SessionIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(sessionIDKey).(string)
	return v, ok
}

type Agent struct {
	provider          LLMProvider
	systemInstruction string
	functionsMap      map[string]*FunctionDeclaration
	sessionRepository repository.SessionRepository
}

type FunctionDeclaration struct {
	Name             string
	Description      string
	ParametersSchema any
	ResponseSchema   any
	FunctionCall     FunctionCallFn
}

type FunctionCallFn func(ctx context.Context, args map[string]any) (map[string]any, error)

func New(provider LLMProvider, systemInstruction string) *Agent {
	return &Agent{
		provider:          provider,
		systemInstruction: systemInstruction,
		functionsMap:      make(map[string]*FunctionDeclaration),
	}
}

func NewWithRepo(provider LLMProvider, systemInstruction string, sessionRepository repository.SessionRepository) *Agent {
	a := New(provider, systemInstruction)
	a.sessionRepository = sessionRepository
	return a
}

func (a *Agent) AddFunctionCall(functionDeclaration *FunctionDeclaration) error {
	if functionDeclaration == nil {
		return fmt.Errorf("function declaration cannot be nil")
	}

	if functionDeclaration.Name == "" {
		return fmt.Errorf("function name cannot be empty")
	}

	if functionDeclaration.FunctionCall == nil {
		return fmt.Errorf("function call implementation cannot be nil")
	}

	a.functionsMap[functionDeclaration.Name] = functionDeclaration

	return nil
}

func (a *Agent) handleFunctionCall(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	if fd, exists := a.functionsMap[name]; exists {
		return fd.FunctionCall(ctx, args)
	}
	return nil, fmt.Errorf("function %s not found", name)
}

func (a *Agent) loadHistory(ctx context.Context, sessionID string) ([]model.Content, error) {
	if a.sessionRepository == nil {
		return nil, nil
	}
	stored, err := a.sessionRepository.Load(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	return stored, nil
}

func (a *Agent) saveHistory(ctx context.Context, sessionID string, history []model.Content) {
	if a.sessionRepository == nil {
		return
	}
	if err := a.sessionRepository.Save(ctx, sessionID, history); err != nil {
		fmt.Printf("agent: warning: failed to save session %q: %v\n", sessionID, err)
	}
}

func (a *Agent) Send(ctx context.Context, sessionID string, prompt string) ([]model.Content, error) {
	ctx = WithSessionID(ctx, sessionID)

	history, err := a.loadHistory(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	req := ProviderRequest{
		SystemInstruction:  a.systemInstruction,
		History:            history,
		Tools:              a.functionsMap,
		HandleFunctionCall: a.handleFunctionCall,
		Prompt:             prompt,
	}

	newContents, err := a.provider.Send(ctx, req)
	if err != nil {
		return nil, err
	}

	a.saveHistory(ctx, sessionID, append(history, newContents...))

	// Return only model response parts (exclude the user message we added)
	modelContents := filterModelContents(newContents)
	return modelContents, nil
}

func (a *Agent) SendStream(ctx context.Context, sessionID string, prompt string, onText func(string) error, onFunctionCall func(name string, args map[string]any) error) error {
	ctx = WithSessionID(ctx, sessionID)

	history, err := a.loadHistory(ctx, sessionID)
	if err != nil {
		return err
	}

	req := ProviderRequest{
		SystemInstruction:  a.systemInstruction,
		History:            history,
		Tools:              a.functionsMap,
		HandleFunctionCall: a.handleFunctionCall,
		Prompt:             prompt,
	}

	newContents, err := a.provider.SendStream(ctx, req, onText, onFunctionCall)
	if err != nil {
		return err
	}

	a.saveHistory(ctx, sessionID, append(history, newContents...))

	return nil
}

func (a *Agent) ClearSession(ctx context.Context, sessionID string) {
	if a.sessionRepository != nil {
		if err := a.sessionRepository.Delete(ctx, sessionID); err != nil {
			fmt.Printf("agent: warning: failed to delete session %q: %v\n", sessionID, err)
		}
	}
}

func (a *Agent) GetSession(ctx context.Context, sessionID string) ([]model.Content, error) {
	if a.sessionRepository == nil {
		return []model.Content{}, nil
	}

	stored, err := a.sessionRepository.Load(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("GetSession: %w", err)
	}

	if stored == nil {
		return []model.Content{}, nil
	}

	return stored, nil
}

// filterModelContents returns only model-role entries from a content slice.
func filterModelContents(contents []model.Content) []model.Content {
	var result []model.Content
	for _, c := range contents {
		if c.Role == "model" {
			result = append(result, c)
		}
	}
	return result
}
