package app

import (
	"context"
	"fmt"

	"github.com/projectTHORN/proton/internal/core/session"
)

// Sessions exposes persisted-session use cases to inbound adapters.
type Sessions struct {
	repository session.Repository
}

func NewSessions(repository session.Repository) *Sessions {
	if repository == nil {
		return nil
	}
	return &Sessions{repository: repository}
}

type SessionListOptions struct {
	WorkspaceKey string
	Prefix       string
	Limit        int
	Offset       int
}

type SessionSummary = session.Summary

func (s *Sessions) ListSummaries(ctx context.Context, options SessionListOptions) ([]SessionSummary, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("session repository is unavailable")
	}
	return s.repository.ListSummaries(ctx, session.ListOptions{
		WorkspaceKey: options.WorkspaceKey,
		Prefix:       options.Prefix,
		Limit:        options.Limit,
		Offset:       options.Offset,
	})
}

func (s *Sessions) Load(ctx context.Context, id string) (session.State, bool, error) {
	if s == nil || s.repository == nil {
		return session.State{}, false, fmt.Errorf("session repository is unavailable")
	}
	return s.repository.Load(ctx, id)
}

func (s *Sessions) Save(ctx context.Context, id string, state session.State) error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("session repository is unavailable")
	}
	return s.repository.Save(ctx, id, state)
}

func (s *Sessions) Delete(ctx context.Context, id string) error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("session repository is unavailable")
	}
	return s.repository.Delete(ctx, id)
}

func (s *Sessions) List(ctx context.Context, prefix string) ([]string, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("session repository is unavailable")
	}
	return s.repository.List(ctx, prefix)
}
