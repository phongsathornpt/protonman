// Package failure defines stable application failure codes and their semantic traits.
package failure

// Code is the stable serialized identifier for an application failure.
type Code string

// Domain identifies the subsystem family responsible for a failure.
type Domain string

const (
	DomainTool       Domain = "tool"
	DomainCommand    Domain = "command"
	DomainWorkspace  Domain = "workspace"
	DomainPermission Domain = "permission"
	DomainRuntime    Domain = "runtime"
	DomainModel      Domain = "model"
	DomainNetwork    Domain = "network"
)

const (
	CodeInvalidArguments           Code = "invalid_arguments"
	CodeCommandFailed              Code = "command_failed"
	CodeInvalidOutput              Code = "invalid_output"
	CodeOutputTooLarge             Code = "output_too_large"
	CodeCanceled                   Code = "canceled"
	CodeDeadlineExceeded           Code = "deadline_exceeded"
	CodePermissionDenied           Code = "permission_denied"
	CodeUnknownTool                Code = "unknown_tool"
	CodeNotFound                   Code = "not_found"
	CodeProtectedPath              Code = "protected_path"
	CodeOutsideWorkspace           Code = "outside_workspace"
	CodeNoProgress                 Code = "no_progress"
	CodeStaleContinuation          Code = "stale_continuation"
	CodeConflict                   Code = "conflict"
	CodePreexistingWorkspaceChange Code = "preexisting_workspace_change"
	CodeWorkspaceStateUnavailable  Code = "workspace_state_unavailable"
	CodeSandboxUnavailable         Code = "sandbox_unavailable"
	CodeExecution                  Code = "execution_error"
	CodeModelInvalidRequest        Code = "model_invalid_request"
	CodeModelAuthentication        Code = "model_authentication_failed"
	CodeModelPermission            Code = "model_permission_denied"
	CodeModelRateLimited           Code = "model_rate_limited"
	CodeModelUnavailable           Code = "model_unavailable"
	CodeModelContextLimit          Code = "model_context_limit"
	CodeModelOverloaded            Code = "model_overloaded"
	CodeModelProtocol              Code = "model_protocol_error"
	CodeNetworkUnavailable         Code = "network_unavailable"
	CodeModelError                 Code = "model_error"
)

// Traits are stable semantics attached to a failure code. UI styling belongs to adapters.
type Traits struct {
	Domain    Domain
	Retryable bool
	Temporary bool
	UserFix   bool
}

var catalog = map[Code]Traits{
	CodeInvalidArguments:           {Domain: DomainTool, UserFix: true},
	CodeCommandFailed:              {Domain: DomainCommand, UserFix: true},
	CodeInvalidOutput:              {Domain: DomainTool},
	CodeOutputTooLarge:             {Domain: DomainTool, UserFix: true},
	CodeCanceled:                   {Domain: DomainRuntime, Retryable: true, Temporary: true},
	CodeDeadlineExceeded:           {Domain: DomainRuntime, Retryable: true, Temporary: true},
	CodePermissionDenied:           {Domain: DomainPermission, UserFix: true},
	CodeUnknownTool:                {Domain: DomainTool, UserFix: true},
	CodeNotFound:                   {Domain: DomainTool, UserFix: true},
	CodeProtectedPath:              {Domain: DomainWorkspace, UserFix: true},
	CodeOutsideWorkspace:           {Domain: DomainWorkspace, UserFix: true},
	CodeNoProgress:                 {Domain: DomainRuntime, UserFix: true},
	CodeStaleContinuation:          {Domain: DomainWorkspace, UserFix: true},
	CodeConflict:                   {Domain: DomainWorkspace, UserFix: true},
	CodePreexistingWorkspaceChange: {Domain: DomainWorkspace, UserFix: true},
	CodeWorkspaceStateUnavailable:  {Domain: DomainWorkspace},
	CodeSandboxUnavailable:         {Domain: DomainRuntime},
	CodeExecution:                  {Domain: DomainRuntime},
	CodeModelInvalidRequest:        {Domain: DomainModel, UserFix: true},
	CodeModelAuthentication:        {Domain: DomainModel, UserFix: true},
	CodeModelPermission:            {Domain: DomainModel, UserFix: true},
	CodeModelRateLimited:           {Domain: DomainModel, Retryable: true, Temporary: true},
	CodeModelUnavailable:           {Domain: DomainModel, UserFix: true},
	CodeModelContextLimit:          {Domain: DomainModel, UserFix: true},
	CodeModelOverloaded:            {Domain: DomainModel, Retryable: true, Temporary: true},
	CodeModelProtocol:              {Domain: DomainModel},
	CodeNetworkUnavailable:         {Domain: DomainNetwork, Retryable: true, Temporary: true},
	CodeModelError:                 {Domain: DomainModel},
}

// TraitsFor returns semantic traits for a known code.
func TraitsFor(code Code) (Traits, bool) {
	traits, ok := catalog[code]
	return traits, ok
}

// Known reports whether code belongs to the stable application failure catalog.
func Known(code Code) bool {
	_, ok := catalog[code]
	return ok
}

// Codes returns all stable codes in deterministic declaration order.
func Codes() []Code {
	return []Code{
		CodeInvalidArguments, CodeCommandFailed, CodeInvalidOutput, CodeOutputTooLarge,
		CodeCanceled, CodeDeadlineExceeded, CodePermissionDenied, CodeUnknownTool,
		CodeNotFound, CodeProtectedPath, CodeOutsideWorkspace, CodeNoProgress,
		CodeStaleContinuation, CodeConflict, CodePreexistingWorkspaceChange,
		CodeWorkspaceStateUnavailable, CodeSandboxUnavailable, CodeExecution,
		CodeModelInvalidRequest, CodeModelAuthentication, CodeModelPermission,
		CodeModelRateLimited, CodeModelUnavailable, CodeModelContextLimit,
		CodeModelOverloaded, CodeModelProtocol, CodeNetworkUnavailable, CodeModelError,
	}
}
