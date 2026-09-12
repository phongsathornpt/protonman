package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/phongsathornpt/protonman/internal/base/failure"
)

// ErrInvalidCall indicates that a call envelope cannot be dispatched safely.
var ErrInvalidCall = errors.New("invalid tool call")

// ErrorCode aliases the centralized stable application failure code.
type ErrorCode = failure.Code

const (
	ErrorCodeInvalidArguments           = failure.CodeInvalidArguments
	ErrorCodeCommandFailed              = failure.CodeCommandFailed
	ErrorCodeInvalidOutput              = failure.CodeInvalidOutput
	ErrorCodeOutputTooLarge             = failure.CodeOutputTooLarge
	ErrorCodeCanceled                   = failure.CodeCanceled
	ErrorCodeDeadlineExceeded           = failure.CodeDeadlineExceeded
	ErrorCodePermissionDenied           = failure.CodePermissionDenied
	ErrorCodeUnknownTool                = failure.CodeUnknownTool
	ErrorCodeNotFound                   = failure.CodeNotFound
	ErrorCodeProtectedPath              = failure.CodeProtectedPath
	ErrorCodeInternalPath               = failure.CodeInternalPath
	ErrorCodeOutsideWorkspace           = failure.CodeOutsideWorkspace
	ErrorCodeNoProgress                 = failure.CodeNoProgress
	ErrorCodeStaleContinuation          = failure.CodeStaleContinuation
	ErrorCodeConflict                   = failure.CodeConflict
	ErrorCodePreexistingWorkspaceChange = failure.CodePreexistingWorkspaceChange
	ErrorCodeWorkspaceStateUnavailable  = failure.CodeWorkspaceStateUnavailable
	ErrorCodeSandboxUnavailable         = failure.CodeSandboxUnavailable
	ErrorCodeExecution                  = failure.CodeExecution
	ErrorCodeNetworkUnavailable         = failure.CodeNetworkUnavailable
)

// RecoveryAction identifies one host-understood deterministic recovery strategy.
type RecoveryAction string

const (
	RecoveryRestartPagination RecoveryAction = "restart_pagination"
	RecoveryRefreshResource   RecoveryAction = "refresh_resource"
	RecoveryDiscoverResource  RecoveryAction = "discover_resource"
	RecoveryUseDedicatedTool  RecoveryAction = "use_dedicated_tool"
)

// Recovery describes a structured suggestion for recovering from a tool failure.
type Recovery struct {
	Action    RecoveryAction  `json:"action"`
	Tool      string          `json:"tool,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolError is an internal error with a stable model-facing classification.
type ToolError struct {
	Code       ErrorCode
	Message    string
	Diagnostic string
	Cause      error
	Recovery   *Recovery
}

// FailureCoder lets adapters classify an error without depending on a concrete
// domain error type. The original error remains available through errors.Is.
type FailureCoder interface {
	FailureCode() ErrorCode
}

func NewToolError(code ErrorCode, message string) *ToolError {
	return &ToolError{
		Code:    code,
		Message: strings.TrimSpace(message),
	}
}

func WrapToolError(code ErrorCode, message string, cause error) *ToolError {
	return &ToolError{
		Code:    code,
		Message: strings.TrimSpace(message),
		Cause:   cause,
	}
}

func (e *ToolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("[%s]: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("[%s]: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *ToolError) WithRecovery(recovery Recovery) *ToolError {
	if e == nil {
		return nil
	}
	e.Recovery = &recovery
	return e
}

func (e *ToolError) WithDiagnostic(diagnostic string) *ToolError {
	if e == nil {
		return nil
	}
	e.Diagnostic = strings.TrimSpace(diagnostic)
	return e
}

func (e *ToolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// RecoveryEvidence is metadata attached to a Failure when a recovery action succeeded.
type RecoveryEvidence struct {
	Action           RecoveryAction  `json:"action"`
	Tool             string          `json:"tool"`
	Output           string          `json:"output,omitempty"`
	StructuredOutput json.RawMessage `json:"structured_output,omitempty"`
	SHA256           string          `json:"sha256,omitempty"`
	Truncated        bool            `json:"truncated,omitempty"`
	Pagination       *Pagination     `json:"pagination,omitempty"`
}

// Failure is the serializable failure portion of a tool result.
type Failure struct {
	Code             ErrorCode         `json:"code"`
	Message          string            `json:"message"`
	Diagnostic       string            `json:"diagnostic,omitempty"`
	Retryable        bool              `json:"retryable,omitempty"`
	Recovery         *Recovery         `json:"recovery,omitempty"`
	RecoveryEvidence *RecoveryEvidence `json:"recovery_evidence,omitempty"`
}

// FailureFromError converts an internal error into a stable result failure.
func FailureFromError(err error) *Failure {
	if err == nil {
		return nil
	}

	result := &Failure{
		Code:    ErrorCodeExecution,
		Message: err.Error(),
	}
	var toolErr *ToolError
	var failureCoder FailureCoder
	switch {
	case errors.As(err, &toolErr):
		result.Code = toolErr.Code
		result.Message = toolErr.Message
		result.Diagnostic = toolErr.Diagnostic
		result.Recovery = toolErr.Recovery
	case errors.As(err, &failureCoder):
		result.Code = failureCoder.FailureCode()
	case errors.Is(err, ErrInvalidCall):
		result.Code = ErrorCodeInvalidArguments
	case errors.Is(err, context.Canceled):
		result.Code = ErrorCodeCanceled
	case errors.Is(err, context.DeadlineExceeded):
		result.Code = ErrorCodeDeadlineExceeded
	case errors.Is(err, os.ErrNotExist):
		result.Code = ErrorCodeNotFound
	case errors.Is(err, os.ErrPermission):
		result.Code = ErrorCodePermissionDenied
	}
	if traits, ok := failure.TraitsFor(result.Code); ok {
		result.Retryable = traits.Retryable
	}
	return result
}
