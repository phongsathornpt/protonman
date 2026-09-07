// Package tools contains the first host-side tool adapters for Proton.
package builtin

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/core/workspace"
	"github.com/projectTHORN/proton/internal/platform/checkpoint"
	"github.com/projectTHORN/proton/internal/platform/sandbox"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// ErrDuplicateTool indicates that a name is already registered.
var ErrDuplicateTool = errors.New("duplicate tool")

// compiledToolValidators holds immutable schema validators produced at registration.
type compiledToolValidators struct {
	input  *sdk.ToolSchemaValidator
	output *sdk.ToolSchemaValidator
}

// Registry is a concurrency-safe in-process tool registry.
type Registry struct {
	mu         sync.RWMutex
	handlers   map[string]tool.Handler
	validators map[string]compiledToolValidators
	order      []string
}

// NewRegistry creates an empty registry and optionally registers handlers.
func NewRegistry(handlers ...tool.Handler) (*Registry, error) {
	registry := &Registry{
		handlers:   make(map[string]tool.Handler),
		validators: make(map[string]compiledToolValidators),
		order:      make([]string, 0, len(handlers)),
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
	stores            []checkpoint.Store
	launcher          sandbox.Launcher
	sandboxConfigured bool
	additional        []tool.Handler
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
// The launcher must be non-nil; use an Off-profile OSLauncher for explicit
// opt-out rather than omitting this option.
func WithSandbox(launcher sandbox.Launcher) RegistryOption {
	return func(options *registryOptions) error {
		if launcher == nil {
			return fmt.Errorf("sandbox launcher is required (use an explicit off-profile launcher to opt out)")
		}
		options.launcher = launcher
		options.sandboxConfigured = true
		return nil
	}
}

// WithAdditionalHandlers appends feature-specific adapters without coupling the
// builtin registry to their owning subsystem.
func WithAdditionalHandlers(handlers ...tool.Handler) RegistryOption {
	return func(options *registryOptions) error {
		for _, handler := range handlers {
			if handler == nil {
				return fmt.Errorf("additional tool handler is required")
			}
			options.additional = append(options.additional, handler)
		}
		return nil
	}
}

// NewDefaultRegistry creates the default workspace-aware coding tool set.
// WithSandbox (explicit launcher, even for Off) and WithCheckpointStore are
// required; omitting them fails closed instead of silently running unconfined
// or without backups.
func NewDefaultRegistry(workspaceRoot *workspace.Workspace, options ...RegistryOption) (*Registry, error) {
	if workspaceRoot == nil {
		return nil, fmt.Errorf("create default registry: workspace is required")
	}
	cfg := registryOptions{
		stores: make([]checkpoint.Store, 0),
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&cfg); err != nil {
			return nil, fmt.Errorf("create default registry: %w", err)
		}
	}
	if !cfg.sandboxConfigured || cfg.launcher == nil {
		return nil, fmt.Errorf("create default registry: WithSandbox with an explicit launcher is required (use an off-profile launcher to opt out)")
	}
	if len(cfg.stores) == 0 {
		return nil, fmt.Errorf("create default registry: WithCheckpointStore is required")
	}
	checkpointStore := selectCheckpointStore(cfg.stores)
	handlers := []tool.Handler{
		NewReadFile(workspaceRoot),
		NewBashWithCheckpoint(workspaceRoot, cfg.launcher, checkpointStore),
		NewWriteFile(workspaceRoot, checkpointStore),
		NewSearchReplace(workspaceRoot, checkpointStore),
		NewApplyPatch(workspaceRoot, checkpointStore),
		NewGrep(workspaceRoot),
		NewFindFiles(workspaceRoot),
		NewListDir(workspaceRoot),
		NewGitStatus(workspaceRoot, cfg.launcher),
		NewCheckpointRestore(checkpointStore),
	}
	handlers = append(handlers, cfg.additional...)
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
	sdkTool := sdk.Tool{Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema, OutputSchema: definition.OutputSchema}
	inputValidator, err := sdk.CompileToolInputValidator(sdkTool)
	if err != nil {
		return fmt.Errorf("register %q input schema: %w", definition.Name, err)
	}
	outputValidator, err := sdk.CompileToolOutputValidator(sdkTool)
	if err != nil {
		return fmt.Errorf("register %q output schema: %w", definition.Name, err)
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
	if r.validators == nil {
		r.validators = make(map[string]compiledToolValidators)
	}
	if _, exists := r.handlers[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateTool, name)
	}
	r.handlers[name] = handler
	r.validators[name] = compiledToolValidators{input: inputValidator, output: outputValidator}
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

// CompiledValidators returns the immutable validators compiled when the tool was registered.
func (r *Registry) CompiledValidators(name string) (input, output *sdk.ToolSchemaValidator, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	validators, ok := r.validators[name]
	if !ok {
		return nil, nil, false
	}
	return validators.input, validators.output, true
}

// Definitions returns a stable registration-order snapshot.
func (r *Registry) Definitions() []tool.Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]tool.Definition, 0, len(r.order))
	for _, name := range r.order {
		definition := r.handlers[name].Definition()
		definition.InputSchema = cloneSchema(definition.InputSchema)
		definition.OutputSchema = cloneSchema(definition.OutputSchema)
		definitions = append(definitions, definition)
	}
	return definitions
}

func cloneSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{}
	}
	data, err := json.Marshal(schema)
	if err == nil {
		var clone map[string]any
		if json.Unmarshal(data, &clone) == nil {
			return clone
		}
	}
	clone := make(map[string]any, len(schema))
	for key, value := range schema {
		clone[key] = value
	}
	return clone
}

var _ tool.Registry = (*Registry)(nil)
var _ tool.Registrar = (*Registry)(nil)
