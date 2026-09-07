package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/projectTHORN/proton/internal/core/tool"
)

// ManagedServer is an MCP server whose transport owns resources that must be closed.
type ManagedServer interface {
	Server
	Close() error
}

// Manager owns a set of MCP server transports for one session.
type Manager struct {
	mu      sync.Mutex
	servers []ManagedServer
	closed  bool
}

func NewManager(servers ...ManagedServer) (*Manager, error) {
	manager := &Manager{servers: make([]ManagedServer, 0, len(servers))}
	seen := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		if server == nil {
			return nil, errors.New("create MCP manager: server is required")
		}
		name := server.Name()
		if _, err := validServerName(name); err != nil {
			return nil, fmt.Errorf("create MCP manager: %w", err)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("create MCP manager: duplicate server %q", name)
		}
		seen[name] = struct{}{}
		manager.servers = append(manager.servers, server)
	}
	return manager, nil
}

// Bind discovers and registers all managed server tools. Failed discovery closes every server.
func (m *Manager) Bind(ctx context.Context, registry tool.Registrar) error {
	if registry == nil {
		return errors.New("bind MCP manager: registry is required")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("bind MCP manager: manager is closed")
	}
	servers := append([]ManagedServer(nil), m.servers...)
	m.mu.Unlock()
	plain := make([]Server, len(servers))
	for i, server := range servers {
		plain[i] = server
	}
	if err := Discover(ctx, registry, plain...); err != nil {
		_ = m.Close()
		return err
	}
	return nil
}

// Close releases all transports in reverse creation order. It is idempotent.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	servers := append([]ManagedServer(nil), m.servers...)
	m.mu.Unlock()
	var errs []error
	for i := len(servers) - 1; i >= 0; i-- {
		if err := servers[i].Close(); err != nil {
			errs = append(errs, fmt.Errorf("close MCP server %q: %w", servers[i].Name(), err))
		}
	}
	return errors.Join(errs...)
}
