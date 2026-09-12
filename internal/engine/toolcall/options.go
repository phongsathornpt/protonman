package toolcall

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
)

// PermissionPrompt resolves an interactive permission request.
type PermissionPrompt func(context.Context, permission.Request) (permission.Resolution, error)

// CallGuard can fail closed before static policy, grants, or permission mode
// evaluation. Adapters use it for temporary execution constraints such as a
// read-only planning mode without weakening the shared policy layer.
type CallGuard func(context.Context, permission.Request) error

// ErrInvalidService indicates that an application service dependency is
// missing or invalid.
var ErrInvalidService = errors.New("invalid tool-call service")

// ErrUnknownTool indicates that a call names no registered tool.
var ErrUnknownTool = errors.New("unknown tool")

// ErrPermissionDenied indicates that a call was stopped before execution.
var ErrPermissionDenied = errors.New("permission denied")

// Option configures a Service during construction.
type Option func(*Service) error

const (
	// DefaultPermissionTimeout bounds policy evaluation and interactive
	// permission resolution when callers do not provide a stricter timeout.
	DefaultPermissionTimeout = runtimepolicy.ToolPermissionTimeout
	// DefaultExecutionTimeout bounds one permission-approved tool call when the
	// caller does not provide a stricter context.
	DefaultExecutionTimeout = runtimepolicy.ToolExecutionTimeout
)

// WithMode sets the initial permission mode.
func WithMode(mode permission.Mode) Option {
	return func(service *Service) error {
		if !mode.Valid() {
			return fmt.Errorf("%w: invalid permission mode %q", ErrInvalidService, mode)
		}
		service.mode = mode
		return nil
	}
}

// WithObserver attaches a redacted lifecycle event observer.
func WithObserver(observer Observer) Option {
	return func(service *Service) error {
		if observer == nil {
			return fmt.Errorf("%w: observer is required", ErrInvalidService)
		}
		service.observer = observer
		return nil
	}
}

// WithPrompt sets the interactive resolver used by ask and auto modes.
func WithPrompt(prompt PermissionPrompt) Option {
	return func(service *Service) error {
		service.prompt = prompt
		return nil
	}
}

// WithPermissionTimeout bounds policy evaluation and interactive permission
// resolution. Zero disables this service-level bound.
func WithPermissionTimeout(timeout time.Duration) Option {
	return func(service *Service) error {
		if timeout < 0 {
			return fmt.Errorf("%w: permission timeout cannot be negative", ErrInvalidService)
		}
		service.permissionTimeout = timeout
		return nil
	}
}

// WithWorkspaceMutationGate serializes mutating calls that target the same workspace instance.
func WithWorkspaceMutationGate(ws *workspace.Workspace) Option {
	return func(service *Service) error {
		if ws == nil {
			return fmt.Errorf("%w: workspace mutation gate requires a workspace", ErrInvalidService)
		}
		service.mutationWorkspace = ws
		return nil
	}
}

// WithExecutionTimeout bounds approved handler execution.
func WithExecutionTimeout(timeout time.Duration) Option {
	return func(service *Service) error {
		if timeout < 0 {
			return fmt.Errorf("%w: execution timeout cannot be negative", ErrInvalidService)
		}
		service.executionTimeout = timeout
		return nil
	}
}

// WithCallGuard attaches an execution guard.
func WithCallGuard(guard CallGuard) Option {
	return func(service *Service) error {
		service.guard = guard
		return nil
	}
}
