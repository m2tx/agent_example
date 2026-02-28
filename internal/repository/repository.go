package repository

import (
	"context"

	"github.com/m2tx/agent_example/internal/model"
)

// SessionRepository defines persistence operations for conversation history.
type SessionRepository interface {
	// Save persists the full history for a given session.
	// Replaces any previously stored history for that sessionID.
	Save(ctx context.Context, sessionID string, history []model.Content) error

	// Load retrieves the stored history for a given session.
	// Returns nil, nil if the session does not exist.
	Load(ctx context.Context, sessionID string) ([]model.Content, error)

	// Delete removes the stored history for a given session.
	// Is a no-op if the session does not exist.
	Delete(ctx context.Context, sessionID string) error
}
