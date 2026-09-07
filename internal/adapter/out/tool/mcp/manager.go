package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/projectTHORN/proton/internal/core/tool"
)

// ManagedServer is an MCP server whose transport owns resources that must be closed.
type ManagedServer interface {
	Server
	Close() error
}

// Manager owns a set of MCP server transports for one session.
type Manager struct {
	mu            sync.Mutex
	servers       []ManagedServer
	closed        bool
	discovery     DiscoveryOptions
	registry      tool.DynamicRegistrar
	generations   map[string]uint64
	changeCh      chan string
	refreshCancel context.CancelFunc
	refreshWG     sync.WaitGroup
	refreshMu     sync.Mutex
}

const toolListChangeDebounce = 50 * time.Millisecond

func NewManager(servers ...ManagedServer) (*Manager, error) {
	return NewManagerWithDiscoveryOptions(DefaultDiscoveryOptions(), servers...)
}

func NewManagerWithDiscoveryOptions(options DiscoveryOptions, servers ...ManagedServer) (*Manager, error) {
	if err := options.Limits.validate(); err != nil {
		return nil, fmt.Errorf("create MCP manager: %w", err)
	}
	manager := &Manager{
		servers: make([]ManagedServer, 0, len(servers)), discovery: options,
		generations: make(map[string]uint64, len(servers)),
		changeCh:    make(chan string, maxInt(1, len(servers)*2)),
	}
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
func (m *Manager) Bind(ctx context.Context, registry tool.DynamicRegistrar) error {
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
	if err := DiscoverWithOptions(ctx, registry, m.discovery, plain...); err != nil {
		_ = m.Close()
		return err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("bind MCP manager: manager closed during discovery")
	}
	m.registry = registry
	for _, server := range servers {
		m.generations[server.Name()] = 1
	}
	refreshCtx, refreshCancel := context.WithCancel(context.Background())
	m.refreshCancel = refreshCancel
	m.refreshWG.Add(1)
	go m.watchToolListChanges(refreshCtx)
	for _, server := range servers {
		if source, ok := server.(ToolListChangeSource); ok {
			name := server.Name()
			source.SetToolListChangedHandler(func() { m.queueToolListChange(name) })
		}
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) queueToolListChange(serverName string) {
	select {
	case m.changeCh <- serverName:
	default:
	}
}

func (m *Manager) watchToolListChanges(ctx context.Context) {
	defer m.refreshWG.Done()
	pending := make(map[string]struct{})
	for {
		select {
		case <-ctx.Done():
			return
		case name := <-m.changeCh:
			pending[name] = struct{}{}
		}
		timer := time.NewTimer(toolListChangeDebounce)
	drain:
		for {
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case name := <-m.changeCh:
				pending[name] = struct{}{}
			case <-timer.C:
				break drain
			}
		}
		for name := range pending {
			_ = m.Refresh(ctx, name)
			delete(pending, name)
		}
	}
}

// CatalogGeneration returns the committed catalog generation for one server.
func (m *Manager) CatalogGeneration(serverName string) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.generations[serverName]
}

// Refresh replaces one server namespace from a newly validated tools/list snapshot.
// A failed refresh leaves the previous catalog intact.
func (m *Manager) Refresh(ctx context.Context, serverName string) error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("refresh MCP catalog: manager is closed")
	}
	registry := m.registry
	options := m.discovery
	nextGeneration := m.generations[serverName] + 1
	var server ManagedServer
	for _, candidate := range m.servers {
		if candidate.Name() == serverName {
			server = candidate
			break
		}
	}
	m.mu.Unlock()
	if registry == nil {
		return errors.New("refresh MCP catalog: manager is not bound")
	}
	if server == nil {
		return fmt.Errorf("refresh MCP catalog: unknown server %q", serverName)
	}
	manifests, err := server.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("refresh MCP catalog from %q: %w", serverName, err)
	}
	limits := options.Limits
	if len(manifests) > limits.MaxToolsPerServer {
		return fmt.Errorf("MCP server %q exposed %d tools; limit is %d", serverName, len(manifests), limits.MaxToolsPerServer)
	}
	prefix := "mcp." + serverName + "."
	totalMCP := len(manifests)
	for _, definition := range registry.Definitions() {
		if definition.Kind == tool.KindMCP && !strings.HasPrefix(definition.Name, prefix) {
			totalMCP++
		}
	}
	if totalMCP > limits.MaxTotalTools {
		return fmt.Errorf("MCP servers would expose %d tools; total limit is %d", totalMCP, limits.MaxTotalTools)
	}
	handlers := make([]tool.Handler, 0, len(manifests))
	seen := make(map[string]struct{}, len(manifests))
	callSlots := make(chan struct{}, limits.MaxConcurrentCallsPerServer)
	for _, manifest := range manifests {
		if err := validateManifestLimits(serverName, manifest, limits); err != nil {
			return err
		}
		if err := manifest.Validate(); err != nil {
			return fmt.Errorf("validate MCP tool from %q: %w", serverName, err)
		}
		name, err := NamespacedName(serverName, manifest.Name)
		if err != nil {
			return fmt.Errorf("namespace MCP tool %q: %w", manifest.Name, err)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateDiscoveredTool, name)
		}
		seen[name] = struct{}{}
		handler, err := newHandler(server, serverName, manifest, name, options.TrustServerSafety, callSlots, nextGeneration)
		if err != nil {
			return fmt.Errorf("clone MCP tool %q schemas: %w", name, err)
		}
		handlers = append(handlers, handler)
	}
	if err := registry.ReplaceNamespace(prefix, handlers); err != nil {
		return fmt.Errorf("replace MCP catalog for %q: %w", serverName, err)
	}
	m.mu.Lock()
	m.generations[serverName] = nextGeneration
	m.mu.Unlock()
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
	refreshCancel := m.refreshCancel
	m.refreshCancel = nil
	m.mu.Unlock()
	if refreshCancel != nil {
		refreshCancel()
		m.refreshWG.Wait()
	}
	var errs []error
	for i := len(servers) - 1; i >= 0; i-- {
		if err := servers[i].Close(); err != nil {
			errs = append(errs, fmt.Errorf("close MCP server %q: %w", servers[i].Name(), err))
		}
	}
	return errors.Join(errs...)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
