// Package tool defines the provider-neutral contract for executable tools.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Kind classifies a tool for permission policy matching.
type Kind string

// Mutability declares whether successful execution can change state that may
// invalidate prior no-progress observations. The zero value preserves legacy
// kind-based inference for third-party tools.
type Mutability string

// ExecutionTimeoutPolicy controls whether the tool-call service adds its own
// execution deadline around a handler. The zero value keeps the service
// default; caller-bound tools rely on the caller/coordinator deadline instead.
type ExecutionTimeoutPolicy string

const (
	ExecutionTimeoutServiceDefault ExecutionTimeoutPolicy = ""
	ExecutionTimeoutCallerBounded  ExecutionTimeoutPolicy = "caller"
)

// CommandEffect classifies the observable state impact of a shell command.
type CommandEffect string

const (
	MutabilityUnspecified Mutability = ""
	MutabilityReadOnly    Mutability = "read_only"
	MutabilityMutating    Mutability = "mutating"
)

const (
	CommandEffectUnknown  CommandEffect = "unknown"
	CommandEffectReadOnly CommandEffect = "read_only"
	CommandEffectMutating CommandEffect = "mutating"
)

const (
	// KindRead identifies tools that read local project state.
	KindRead Kind = "read"
	// KindEdit identifies tools that mutate project files.
	KindEdit Kind = "edit"
	// KindBash identifies tools that execute shell commands.
	KindBash Kind = "bash"
	// KindGrep identifies tools that search project contents.
	KindGrep Kind = "grep"
	// KindMCP identifies tools provided by an MCP server.
	KindMCP Kind = "mcp"
	// KindWebFetch identifies tools that fetch a URL.
	KindWebFetch Kind = "web_fetch"
	// KindWebSearch identifies tools that search the web.
	KindWebSearch Kind = "web_search"
	// KindTask identifies structured planning/task metadata mutations.
	KindTask Kind = "task"
	// KindAgent identifies subagent orchestration and lifecycle tools.
	KindAgent Kind = "agent"
)

// ErrInvalidCall indicates that a call envelope cannot be dispatched safely.
var ErrInvalidCall = errors.New("invalid tool call")

// ErrorCode classifies failures for model and headless clients.
type ErrorCode string

const (
	// ErrorCodeInvalidArguments indicates that a call cannot be decoded or validated.
	ErrorCodeInvalidArguments ErrorCode = "invalid_arguments"
	// ErrorCodeCanceled indicates that the caller canceled execution.
	ErrorCodeCanceled ErrorCode = "canceled"
	// ErrorCodeDeadlineExceeded indicates that the call exceeded its deadline.
	ErrorCodeDeadlineExceeded ErrorCode = "deadline_exceeded"
	// ErrorCodePermissionDenied indicates that policy stopped the call.
	ErrorCodePermissionDenied ErrorCode = "permission_denied"
	// ErrorCodeUnknownTool indicates that no handler is registered for a call.
	ErrorCodeUnknownTool ErrorCode = "unknown_tool"
	// ErrorCodeNotFound indicates that a requested target does not exist.
	ErrorCodeNotFound ErrorCode = "not_found"
	// ErrorCodeProtectedPath indicates that a workspace target is protected.
	ErrorCodeProtectedPath ErrorCode = "protected_path"
	// ErrorCodeOutsideWorkspace indicates that a path escaped the workspace.
	ErrorCodeOutsideWorkspace ErrorCode = "outside_workspace"
	// ErrorCodeNoProgress indicates that loop protection suppressed a repeated call.
	ErrorCodeNoProgress ErrorCode = "no_progress"
	// ErrorCodeStaleContinuation indicates that pageable state changed since the previous page.
	ErrorCodeStaleContinuation ErrorCode = "stale_continuation"
	// ErrorCodeConflict indicates an optimistic concurrency/version conflict.
	ErrorCodeConflict ErrorCode = "conflict"
	// ErrorCodeSandboxUnavailable indicates that requested OS confinement could not be applied.
	ErrorCodeSandboxUnavailable ErrorCode = "sandbox_unavailable"
	// ErrorCodeExecution is the safe fallback for handler failures.
	ErrorCodeExecution ErrorCode = "execution_error"
)

// ToolError is an internal error with a stable model-facing classification.
type ToolError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

// FailureCoder lets adapters classify an error without depending on a concrete
// domain error type. The original error remains available through errors.Is.
type FailureCoder interface {
	FailureCode() ErrorCode
}

// NewToolError creates a classified tool error without an underlying cause.
func NewToolError(code ErrorCode, message string) *ToolError {
	return &ToolError{
		Code:    code,
		Message: strings.TrimSpace(message),
	}
}

// WrapToolError creates a classified tool error while preserving its cause.
func WrapToolError(code ErrorCode, message string, cause error) *ToolError {
	return &ToolError{
		Code:    code,
		Message: strings.TrimSpace(message),
		Cause:   cause,
	}
}

// Error implements error.
func (e *ToolError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message == "" && e.Cause != nil {
		return e.Cause.Error()
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

// Unwrap exposes the underlying cause to errors.Is and errors.As.
func (e *ToolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Failure is the serializable failure portion of a tool result.
type Failure struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable,omitempty"`
}

// FailureFromError converts an internal error into a stable result failure.
func FailureFromError(err error) *Failure {
	if err == nil {
		return nil
	}

	failure := &Failure{
		Code:    ErrorCodeExecution,
		Message: err.Error(),
	}
	var toolErr *ToolError
	var failureCoder FailureCoder
	switch {
	case errors.As(err, &toolErr):
		failure.Code = toolErr.Code
	case errors.As(err, &failureCoder):
		failure.Code = failureCoder.FailureCode()
	case errors.Is(err, ErrInvalidCall):
		failure.Code = ErrorCodeInvalidArguments
	case errors.Is(err, context.Canceled):
		failure.Code = ErrorCodeCanceled
		failure.Retryable = true
	case errors.Is(err, context.DeadlineExceeded):
		failure.Code = ErrorCodeDeadlineExceeded
		failure.Retryable = true
	}
	return failure
}

// Call is the JSON-typed envelope passed from a model or UI to a tool.
type Call struct {
	// ID uniquely identifies the call within a turn or session.
	ID string
	// Name is the registered tool name.
	Name string
	// Arguments contains a JSON object accepted by the tool.
	Arguments json.RawMessage
}

// NewCall validates and copies a tool call envelope.
func NewCall(id string, name string, arguments json.RawMessage) (Call, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" {
		return Call{}, fmt.Errorf("%w: call id is required", ErrInvalidCall)
	}
	if name == "" {
		return Call{}, fmt.Errorf("%w: tool name is required", ErrInvalidCall)
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(arguments) {
		return Call{}, fmt.Errorf("%w: arguments must be valid JSON", ErrInvalidCall)
	}

	return Call{
		ID:        id,
		Name:      name,
		Arguments: append(json.RawMessage(nil), arguments...),
	}, nil
}

// Validate checks a call that may have been built as a struct literal.
func (c Call) Validate() error {
	_, err := NewCall(c.ID, c.Name, c.Arguments)
	return err
}

// Definition describes a registered tool to the model and the UI.
type Definition struct {
	// Name is the stable dispatch name.
	Name string
	// Description explains the tool's purpose to a model or user.
	Description string
	// Kind is the permission category for this tool.
	Kind Kind
	// Mutability declares whether successful execution can invalidate prior
	// read observations. Unspecified preserves legacy kind-based inference.
	Mutability Mutability
	// ExecutionTimeoutPolicy controls whether the service applies its generic
	// per-tool execution timeout. Orchestration tools may instead rely on a
	// stricter caller/coordinator deadline.
	ExecutionTimeoutPolicy ExecutionTimeoutPolicy
	// PermissionDetailKey names the JSON argument shown to a permission
	// prompt and matched by path, command, or domain rules.
	PermissionDetailKey string
	// InputSchema is the JSON-schema-like manifest exposed to callers.
	InputSchema map[string]any
}

// Validate checks the invariants required for safe registry insertion.
func (d Definition) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("%w: tool name is required", ErrInvalidCall)
	}
	if strings.TrimSpace(d.Description) == "" {
		return fmt.Errorf("%w: description is required for %q", ErrInvalidCall, d.Name)
	}
	if !validKind(d.Kind) {
		return fmt.Errorf("%w: unsupported kind %q for %q", ErrInvalidCall, d.Kind, d.Name)
	}
	if !validMutability(d.Mutability) {
		return fmt.Errorf("%w: unsupported mutability %q for %q", ErrInvalidCall, d.Mutability, d.Name)
	}
	if !validExecutionTimeoutPolicy(d.ExecutionTimeoutPolicy) {
		return fmt.Errorf("%w: unsupported execution timeout policy %q for %q", ErrInvalidCall, d.ExecutionTimeoutPolicy, d.Name)
	}
	return nil
}

// Result is the model-facing output of a tool execution.
type Result struct {
	// CallID identifies the originating call.
	CallID string `json:"call_id"`
	// ToolName identifies the handler that produced the result.
	ToolName string `json:"tool_name"`
	// Output is the compatibility view presented to existing model/UI adapters.
	Output string `json:"output,omitempty"`
	// Stdout and Stderr preserve process streams separately when available.
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
	// StdoutBytes and StderrBytes count bytes observed before truncation.
	StdoutBytes int64 `json:"stdout_bytes,omitempty"`
	StderrBytes int64 `json:"stderr_bytes,omitempty"`
	// Stream-specific truncation flags preserve which channel exceeded its cap.
	StdoutTruncated bool `json:"stdout_truncated,omitempty"`
	StderrTruncated bool `json:"stderr_truncated,omitempty"`
	// ExitCode is populated by process-backed tools when a process exits.
	ExitCode *int `json:"exit_code,omitempty"`
	// Denied reports that execution was blocked before the handler ran.
	Denied bool `json:"denied,omitempty"`
	// Truncated reports that an output limit shortened the result.
	Truncated bool `json:"truncated,omitempty"`
	// NextOffset is the continuation offset for pageable tools when Truncated is true.
	NextOffset *int64 `json:"next_offset,omitempty"`
	// Continuation binds a truncated page to its query and filesystem snapshot.
	Continuation string `json:"continuation,omitempty"`
	// Failure is populated when a tool call fails or is denied.
	Failure *Failure `json:"error,omitempty"`
	// CheckpointID identifies the pre-edit snapshot created by a mutating tool.
	CheckpointID string `json:"checkpoint_id,omitempty"`
	// AffectedPaths lists workspace-relative paths successfully mutated by the tool.
	AffectedPaths []string `json:"affected_paths,omitempty"`
}

// Handler executes one registered tool call.
type Handler interface {
	// Definition returns the stable manifest and permission category.
	Definition() Definition
	// Execute performs the tool's external effect, honoring context
	// cancellation where the underlying operation supports it.
	Execute(ctx context.Context, call Call) (Result, error)
}

// DetailProvider allows a handler to produce a customized detail string
// for permission evaluation and user prompts (e.g. summarizing affected files).
type DetailProvider interface {
	PermissionDetail(arguments json.RawMessage) string
}

// Registry resolves tool names and publishes their definitions.
type Registry interface {
	// Lookup returns the handler registered under name.
	Lookup(name string) (Handler, bool)
	// Definitions returns a stable snapshot of registered tool manifests.
	Definitions() []Definition
}

// Registrar extends a registry with safe handler registration for discovery adapters.
type Registrar interface {
	Registry
	Register(Handler) error
}

func validKind(kind Kind) bool {
	switch kind {
	case KindRead, KindEdit, KindBash, KindGrep, KindMCP, KindWebFetch, KindWebSearch, KindTask, KindAgent:
		return true
	default:
		return false
	}
}

func validExecutionTimeoutPolicy(policy ExecutionTimeoutPolicy) bool {
	switch policy {
	case ExecutionTimeoutServiceDefault, ExecutionTimeoutCallerBounded:
		return true
	default:
		return false
	}
}

func validMutability(mutability Mutability) bool {
	switch mutability {
	case MutabilityUnspecified, MutabilityReadOnly, MutabilityMutating:
		return true
	default:
		return false
	}
}

// EffectiveMutability resolves explicit metadata while preserving behavior for
// tools compiled before mutability metadata existed.
func EffectiveMutability(definition Definition) Mutability {
	if definition.Mutability != MutabilityUnspecified {
		return definition.Mutability
	}
	switch definition.Kind {
	case KindRead, KindGrep, KindWebFetch, KindWebSearch:
		return MutabilityReadOnly
	default:
		return MutabilityMutating
	}
}

// EffectiveCallMutability refines a definition's static metadata with safe
// per-call knowledge when available. Unknown shell effects remain mutating.
func EffectiveCallMutability(definition Definition, arguments json.RawMessage) Mutability {
	if definition.Kind != KindBash {
		return EffectiveMutability(definition)
	}
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return MutabilityMutating
	}
	switch AnalyzeCommand(input.Command).Effect {
	case CommandEffectReadOnly:
		return MutabilityReadOnly
	default:
		return MutabilityMutating
	}
}
