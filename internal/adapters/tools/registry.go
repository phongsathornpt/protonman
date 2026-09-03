// Package tools contains the first host-side tool adapters for Proton.
package tools

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/projectTHORN/proton/internal/adapters/workspace"
	"github.com/projectTHORN/proton/internal/checkpoint"
	"github.com/projectTHORN/proton/internal/domain/tool"
	"github.com/projectTHORN/proton/internal/sandbox"
	"github.com/projectTHORN/proton/internal/skill"
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

// RegistryOption configures the default coding-tool set.
type RegistryOption func(*registryOptions) error

type registryOptions struct {
	stores   []checkpoint.Store
	launcher sandbox.Launcher
	network  sandbox.NetworkPolicy
	skills   *skill.Registry
}

// WithCheckpointStore attaches durable edit checkpoints.
func WithCheckpointStore(store checkpoint.Store) RegistryOption {
	return func(options *registryOptions) error {
		if store == nil {
			return fmt.Errorf("checkpoint store is required")
		}
		options.stores = append(options.stores, store)
		return nil
	}
}

// WithSandbox confines bash and web_fetch under the resolved profile.
func WithSandbox(launcher sandbox.Launcher, network sandbox.NetworkPolicy) RegistryOption {
	return func(options *registryOptions) error {
		options.launcher = launcher
		options.network = network
		return nil
	}
}

// WithSkillRegistry attaches an Agent Skill registry and registers activate_skill.
func WithSkillRegistry(registry *skill.Registry) RegistryOption {
	return func(options *registryOptions) error {
		options.skills = registry
		return nil
	}
}

// NewDefaultRegistry creates the default workspace-aware coding tool set.
func NewDefaultRegistry(workspaceRoot *workspace.Workspace, options ...RegistryOption) (*Registry, error) {
	if workspaceRoot == nil {
		return nil, fmt.Errorf("create default registry: workspace is required")
	}
	cfg := registryOptions{
		stores: make([]checkpoint.Store, 0),
		network: sandbox.NetworkPolicy{
			Mode:    sandbox.NetworkUnrestricted,
			Allowed: []sandbox.Origin{},
		},
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&cfg); err != nil {
			return nil, fmt.Errorf("create default registry: %w", err)
		}
	}
	checkpointStore := selectCheckpointStore(cfg.stores)
	handlers := []tool.Handler{
		NewReadFile(workspaceRoot),
		NewBash(workspaceRoot, cfg.launcher),
		NewWriteFile(workspaceRoot, checkpointStore),
		NewSearchReplace(workspaceRoot, checkpointStore),
		NewApplyPatch(workspaceRoot, checkpointStore),
		NewGrep(workspaceRoot),
		NewListDir(workspaceRoot),
		NewGitStatus(workspaceRoot),
		NewCheckpointRestore(checkpointStore),
		NewWebFetch(cfg.network),
	}
	if cfg.skills != nil {
		handlers = append(handlers, NewActivateSkill(cfg.skills))
	}
	return NewRegistry(handlers...)
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
var _ tool.Registrar = (*Registry)(nil)
