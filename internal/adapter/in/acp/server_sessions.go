package acp

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func (s *Server) lookupSession(sessionID string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	return sess, ok
}

func (s *Server) loadOrCreateSession(ctx context.Context, sessionID string, cwd string, additionalDirectories []string, mcpSets ...[]MCPServerConfig) (*Session, error) {
	var mcpServers []MCPServerConfig
	if len(mcpSets) > 0 {
		mcpServers = mcpSets[0]
	}
	s.mu.Lock()
	existing, ok := s.sessions[sessionID]
	currentDirectories := cloneDirectories(s.sessionDirectories[sessionID])
	s.mu.Unlock()
	if ok {
		if cwd != "" && existing.cwd != "" && cwd != existing.cwd {
			return nil, fmt.Errorf("session %q belongs to cwd %q, not %q", sessionID, existing.cwd, cwd)
		}
		if err := existing.matchMCPServers(mcpServers); err != nil {
			return nil, err
		}
		if sameDirectories(currentDirectories, additionalDirectories) {
			return existing, nil
		}
		if sessionIsActive(existing) {
			return nil, fmt.Errorf("session %q has an active prompt; workspace roots cannot change", sessionID)
		}
		if _, err := s.agents.ForSession(sessionID).CancelSessionAndWait(ctx); err != nil {
			return nil, fmt.Errorf("cancel session subagents before workspace reconfiguration %q: %w", sessionID, err)
		}
		if err := existing.Close(); err != nil {
			return nil, fmt.Errorf("close session %q before workspace reconfiguration: %w", sessionID, err)
		}
		s.mu.Lock()
		delete(s.sessions, sessionID)
		delete(s.sessionDirectories, sessionID)
		s.mu.Unlock()
	}

	sess, err := s.newSession(ctx, sessionID, cwd, additionalDirectories, mcpServers)
	if err != nil {
		return nil, err
	}
	if s.sessionService != nil {
		state, found, err := s.sessionService.Load(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("load session state %q: %w", sessionID, err)
		}
		if found {
			if cwd != "" && state.WorkspaceKey != "" && state.WorkspaceKey != session.WorkspaceKey(cwd) {
				return nil, fmt.Errorf("session %q belongs to another workspace", sessionID)
			}
			if state.WorkspaceKey != "" {
				sess.workspaceKey = state.WorkspaceKey
			}
			if state.WorkspaceName != "" {
				sess.workspaceName = state.WorkspaceName
			}
			sess.stateRevision = state.Revision
			sess.SetMessages(session.ToModelMessages(state.Messages))
			mode, err := permission.ParseMode(state.PermissionMode)
			if err != nil {
				return nil, fmt.Errorf("session permission mode %q: %w", sessionID, err)
			}
			if err := sess.service.SetMode(mode); err != nil {
				return nil, fmt.Errorf("restore session mode %q: %w", sessionID, err)
			}
			if strings.TrimSpace(state.ReasoningEffort) != "" {
				effort, parseErr := domain.ParseReasoningEffort(state.ReasoningEffort)
				if parseErr != nil {
					return nil, fmt.Errorf("restore session reasoning %q: %w", sessionID, parseErr)
				}
				if err := sess.SetReasoningEffort(effort); err != nil {
					return nil, fmt.Errorf("restore session reasoning %q: %w", sessionID, err)
				}
			}
			if err := restoreSessionRuntime(ctx, s, sess, state); err != nil {
				return nil, fmt.Errorf("restore session runtime %q: %w", sessionID, err)
			}
		}
	}
	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.sessionDirectories[sessionID] = cloneDirectories(additionalDirectories)
	s.mu.Unlock()
	return sess, nil
}

// newSession retains the historic four-argument call shape used by package
// tests while allowing MCP configuration as an optional fifth argument. The
// fourth argument is the ACP full additional-root list; nil preserves the old
// no-extra-roots behavior.
func (s *Server) newSession(ctx context.Context, sessionID string, cwd string, additionalDirectories []string, mcpSets ...[]MCPServerConfig) (*Session, error) {
	var mcpServers []MCPServerConfig
	if len(mcpSets) > 0 {
		mcpServers = mcpSets[0]
	}
	registry := s.registry
	var mcpResource io.Closer
	if s.sessionRegistryFactory != nil {
		created, err := s.sessionRegistryFactory(sessionID, cwd, cloneDirectories(additionalDirectories))
		if err != nil {
			return nil, fmt.Errorf("create registry for session %q: %w", sessionID, err)
		}
		registry = created
	}
	if len(mcpServers) > 0 {
		if s.mcpRegistryConfigurer == nil {
			return nil, fmt.Errorf("configure MCP servers for session %q: MCP server configuration is not available", sessionID)
		}
		resource, err := s.mcpRegistryConfigurer(ctx, cwd, registry, cloneMCPServerConfigs(mcpServers))
		if err != nil {
			return nil, fmt.Errorf("configure MCP servers for session %q: %w", sessionID, err)
		}
		mcpResource = resource
	}
	service, err := s.service.CloneWithRegistry(registry)
	if err != nil {
		if mcpResource != nil {
			_ = mcpResource.Close()
		}
		return nil, fmt.Errorf("%w: clone session tool-call service: %v", ErrInvalidServer, err)
	}
	var runner app.Conversation
	if s.runnerFactory != nil {
		created, err := s.runnerFactory(service)
		if err != nil {
			if mcpResource != nil {
				_ = mcpResource.Close()
			}
			return nil, fmt.Errorf("create runner for session %q: %w", sessionID, err)
		}
		runner = created
	}
	sess := NewSession(sessionID, cwd, service, registry, runner, s.sessionService, s.agents.ForSession(sessionID))
	bindSessionRuntime(s, sess)
	// Without a prompt, ask/auto mode denies every non-statically-allowed call
	// with "no permission prompt is configured". Install the ACP reverse request
	// for this session and its subagents so delegated work asks the same client.
	if s.permissions != nil {
		prompt := s.permissions.prompt(sessionID)
		service.SetPrompt(prompt)
		sess.agents.SetPrompt(prompt)
	}
	sess.mcpServers = cloneMCPServerConfigs(mcpServers)
	sess.resource = mcpResource
	return sess, nil
}

func (s *Server) listSessions(ctx context.Context, cwd string) ([]SessionInfo, error) {
	s.mu.Lock()
	seen := make(map[string]bool)
	activeIndexes := make(map[string]int, len(s.sessions))
	list := make([]SessionInfo, 0, len(s.sessions))
	for id, sess := range s.sessions {
		if cwd != "" && sess.cwd != "" && sess.cwd != cwd {
			continue
		}
		seen[id] = true
		preview := session.Preview(session.FromModelMessages(sess.Messages()))
		activeIndexes[id] = len(list)
		list = append(list, SessionInfo{
			SessionID:             id,
			Cwd:                   sess.cwd,
			AdditionalDirectories: cloneDirectories(s.sessionDirectories[id]),
			Title:                 sessionListTitle(id, sess.workspaceName, preview),
			WorkspaceKey:          sess.workspaceKey,
			WorkspaceName:         sess.workspaceName,
		})
	}
	s.mu.Unlock()
	if s.sessionService != nil {
		options := app.SessionListOptions{}
		if cwd != "" {
			options.WorkspaceKey = session.WorkspaceKey(cwd)
		}
		summaries, err := s.sessionService.ListSummaries(ctx, options)
		if err != nil {
			return nil, fmt.Errorf("list session state: %w", err)
		}
		for _, summary := range summaries {
			if seen[summary.ID] {
				if index, ok := activeIndexes[summary.ID]; ok && !summary.UpdatedAt.IsZero() {
					list[index].UpdatedAt = summary.UpdatedAt.Format(time.RFC3339Nano)
				}
				continue
			}
			list = append(list, SessionInfo{
				SessionID:     summary.ID,
				Cwd:           cwd,
				Title:         sessionListTitle(summary.ID, summary.WorkspaceName, summary.Preview),
				WorkspaceKey:  summary.WorkspaceKey,
				WorkspaceName: summary.WorkspaceName,
				UpdatedAt:     summary.UpdatedAt.Format(time.RFC3339Nano),
			})
		}
	}
	return list, nil
}

func sessionListTitle(id, workspaceName, preview string) string {
	if preview = strings.TrimSpace(preview); preview != "" {
		return preview
	}
	if workspaceName = strings.TrimSpace(workspaceName); workspaceName != "" {
		return workspaceName
	}
	return "Session " + id
}

func (s *Server) closeSession(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if ok {
		delete(s.sessions, sessionID)
		delete(s.sessionDirectories, sessionID)
	}
	s.mu.Unlock()
	if !ok || sess == nil {
		return fmt.Errorf("unknown session %q", sessionID)
	}

	sess.Cancel()
	if _, err := s.agents.ForSession(sessionID).CancelSessionAndWait(ctx); err != nil {
		return fmt.Errorf("cancel session subagents %q: %w", sessionID, err)
	}
	if err := sess.Close(); err != nil {
		return fmt.Errorf("close session %q: %w", sessionID, err)
	}
	return nil
}

func (s *Server) deleteSession(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	_, active := s.sessions[sessionID]
	s.mu.Unlock()

	if s.sessionService != nil {
		if err := s.sessionService.Delete(ctx, sessionID); err != nil {
			return fmt.Errorf("delete session state %q: %w", sessionID, err)
		}
	}
	if active {
		if err := s.closeSession(ctx, sessionID); err != nil {
			return err
		}
		return nil
	}
	if _, err := s.agents.ForSession(sessionID).CancelSessionAndWait(ctx); err != nil {
		return fmt.Errorf("cancel session subagents %q: %w", sessionID, err)
	}
	return nil
}

func (s *Server) closeSessions() {
	s.mu.Lock()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.Unlock()
	for _, sess := range sessions {
		_ = sess.Close()
	}
}
