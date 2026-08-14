// Package tools contains the first host-side tool adapters for Proton.
package tools

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/projectTHORN/proton/internal/adapters/workspace"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

// ErrDuplicateTool indicates that a name is already registered.
var ErrDuplicateTool = errors.New("duplicate tool")

// Registry is a concurrency-safe in-process tool registry.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]tool.Handler
	order    []string
}

// NewRegistry creates an empty registry and optionally registers handlers.
func NewRegistry(handlers ...tool.Handler) (*Registry, error) {
	registry := &Registry{
		handlers: make(map[string]tool.Handler),
		order:    make([]string, 0, len(handlers)),
	}
	for _, handler := range handlers {
		if err := registry.Register(handler); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// NewDefaultRegistry creates the default workspace-aware coding tool set.
func NewDefaultRegistry(workspaceRoot *workspace.Workspace) (*Registry, error) {
	if workspaceRoot == nil {
		return nil, fmt.Errorf("create default registry: workspace is required")
	}
	return NewRegistry(
		NewReadFile(workspaceRoot),
		NewBash(workspaceRoot),
		NewWriteFile(workspaceRoot),
		NewSearchReplace(workspaceRoot),
		NewApplyPatch(workspaceRoot),
		NewGrep(workspaceRoot),
		NewListDir(workspaceRoot),
		NewGitStatus(workspaceRoot),
	)
}

// Register adds a handler to the registry.
func (r *Registry) Register(handler tool.Handler) error {
	if handler == nil {
		return fmt.Errorf("register tool: handler is required")
	}
	definition := handler.Definition()
	if err := definition.Validate(); err != nil {
		return fmt.Errorf("register %q: %w", definition.Name, err)
	}

	name := strings.TrimSpace(definition.Name)
	if definition.Name != name {
		return fmt.Errorf("register tool: name %q must not have leading or trailing whitespace", definition.Name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handlers == nil {
		r.handlers = make(map[string]tool.Handler)
	}
	if _, exists := r.handlers[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateTool, name)
	}
	r.handlers[name] = handler
	r.order = append(r.order, name)
	return nil
}

// Lookup returns a handler without exposing registry internals.
func (r *Registry) Lookup(name string) (tool.Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, ok := r.handlers[name]
	return handler, ok
}

// Definitions returns a stable registration-order snapshot.
func (r *Registry) Definitions() []tool.Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]tool.Definition, 0, len(r.order))
	for _, name := range r.order {
		definition := r.handlers[name].Definition()
		definition.InputSchema = cloneSchema(definition.InputSchema)
		definitions = append(definitions, definition)
	}
	return definitions
}

func cloneSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{}
	}
	clone := make(map[string]any, len(schema))
	for key, value := range schema {
		clone[key] = value
	}
	return clone
}

var _ tool.Registry = (*Registry)(nil)
