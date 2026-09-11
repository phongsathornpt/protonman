// Package tool defines the provider-neutral contract for executable tools.
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

// Kind classifies a tool for permission policy matching.
type Kind string

// Mutability declares whether successful execution can change state that may
// invalidate prior no-progress observations. The zero value preserves legacy
// kind-based inference for third-party tools.
type Mutability string

// MutationDomain identifies which state family a tool may mutate.
type MutationDomain string

// MutationSafety describes how precisely a workspace mutation is scoped.
type MutationSafety string

// MutationCoverage reports how completely a result identifies workspace paths that may have changed.
type MutationCoverage string

// CheckpointPolicy declares whether a workspace mutation must be snapshotted.
type CheckpointPolicy string

// BoundaryPolicy declares the resource boundary enforced by the tool.
type BoundaryPolicy string

// SafetyContract makes host-side safety semantics explicit instead of inferring them from tool names.
type SafetyContract struct {
	MutationDomain   MutationDomain
	MutationSafety   MutationSafety
	CheckpointPolicy CheckpointPolicy
	Boundary         BoundaryPolicy
}

// CallSemantics describes the effective safety and state semantics of one
// concrete invocation. Capability facade tools use this to refine a broad
// static Definition according to an action encoded in the arguments.
type CallSemantics struct {
	Mutability Mutability
	Safety     SafetyContract
	Evidence   EvidenceKind
	Effect     CommandEffect
	Risk       CommandRisk
	Scope      CommandScope
}

// CallSemanticsResolver refines a tool definition for one concrete call.
// Resolvers should start from StaticCallSemantics and change only semantics
// proven by the call arguments.
type CallSemanticsResolver func(json.RawMessage) CallSemantics

// Declared reports whether a tool explicitly published its safety semantics.
func (s SafetyContract) Declared() bool {
	return s.MutationDomain != MutationDomainUnspecified &&
		s.MutationSafety != MutationSafetyUnspecified &&
		s.CheckpointPolicy != CheckpointPolicyUnspecified &&
		s.Boundary != BoundaryPolicyUnspecified
}

// Validate checks cross-field invariants for an explicitly declared safety contract.
func (s SafetyContract) Validate() error {
	if !s.Declared() {
		return fmt.Errorf("safety contract is incomplete")
	}
	if s.MutationDomain != MutationDomainWorkspace && s.MutationSafety != MutationSafetyNone {
		return fmt.Errorf("non-workspace mutation domain cannot declare workspace mutation safety %q", s.MutationSafety)
	}
	if s.MutationDomain != MutationDomainWorkspace && s.CheckpointPolicy != CheckpointPolicyNone {
		return fmt.Errorf("non-workspace mutation domain cannot require checkpoints")
	}
	if s.MutationDomain == MutationDomainNone && s.MutationSafety != MutationSafetyNone {
		return fmt.Errorf("non-mutating tools must use mutation safety none")
	}
	if s.CheckpointPolicy == CheckpointPolicyRequired && s.MutationDomain != MutationDomainWorkspace {
		return fmt.Errorf("required checkpoints are only valid for workspace mutations")
	}
	return nil
}

// ExecutionTimeoutPolicy controls whether the tool-call service adds its own
// execution deadline around a handler. The zero value keeps the service
// default; caller-bound tools rely on the caller/coordinator deadline instead.
type ExecutionTimeoutPolicy string

// EvidenceKind declares which empirical state a successful tool result can
// establish for grounding policy. The zero value intentionally means no
// grounding evidence; evidence must be declared explicitly.
type EvidenceKind string

const (
	ExecutionTimeoutServiceDefault ExecutionTimeoutPolicy = ""
	ExecutionTimeoutCallerBounded  ExecutionTimeoutPolicy = "caller"
)

const (
	EvidenceNone      EvidenceKind = ""
	EvidenceWorkspace EvidenceKind = "workspace"
	EvidenceExternal  EvidenceKind = "external"
)

// CommandEffect classifies the observable state impact of a shell command.
type CommandEffect string

// CommandRisk classifies proven destructive behavior independently from generic mutability.
type CommandRisk string

// CommandScope classifies where a mutating shell command applies its external effect.
type CommandScope string

const (
	MutabilityUnspecified Mutability = ""
	MutabilityReadOnly    Mutability = "read_only"
	MutabilityMutating    Mutability = "mutating"
)

const (
	MutationDomainUnspecified     MutationDomain = ""
	MutationDomainNone            MutationDomain = "none"
	MutationDomainWorkspace       MutationDomain = "workspace"
	MutationDomainTaskState       MutationDomain = "task_state"
	MutationDomainAgentState      MutationDomain = "agent_state"
	MutationDomainWorkspacePolicy MutationDomain = "workspace_policy"
)

const (
	MutationSafetyUnspecified MutationSafety = ""
	MutationSafetyNone        MutationSafety = "none"
	MutationSafetyContextual  MutationSafety = "contextual"
	MutationSafetyWholeFile   MutationSafety = "whole_file"
	MutationSafetyDynamic     MutationSafety = "dynamic"
)

const (
	MutationCoverageNone    MutationCoverage = ""
	MutationCoverageFull    MutationCoverage = "full"
	MutationCoveragePartial MutationCoverage = "partial"
	MutationCoverageUnknown MutationCoverage = "unknown"
)

const (
	CheckpointPolicyUnspecified CheckpointPolicy = ""
	CheckpointPolicyNone        CheckpointPolicy = "none"
	CheckpointPolicyRequired    CheckpointPolicy = "required"
	CheckpointPolicyWhenKnown   CheckpointPolicy = "when_known"
)

const (
	BoundaryPolicyUnspecified    BoundaryPolicy = ""
	BoundaryPolicyNone           BoundaryPolicy = "none"
	BoundaryPolicyWorkspaceRead  BoundaryPolicy = "workspace_read"
	BoundaryPolicyWorkspaceWrite BoundaryPolicy = "workspace_write"
	BoundaryPolicyExternalRead   BoundaryPolicy = "external_read"
	BoundaryPolicySandbox        BoundaryPolicy = "sandbox"
)

const (
	CommandEffectUnknown  CommandEffect = "unknown"
	CommandEffectReadOnly CommandEffect = "read_only"
	CommandEffectMutating CommandEffect = "mutating"
)

const (
	CommandRiskNormal            CommandRisk = ""
	CommandRiskDestructive       CommandRisk = "destructive"
	CommandRiskRemoteDestructive CommandRisk = "remote_destructive"
)

const (
	CommandScopeUnknown    CommandScope = ""
	CommandScopeLocal      CommandScope = "local"
	CommandScopeRemote     CommandScope = "remote"
	CommandScopePublish    CommandScope = "publish"
	CommandScopeDeployment CommandScope = "deployment"
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
	// KindGit identifies repository operations whose mutability is action-dependent.
	KindGit Kind = "git"
	// KindMCP identifies tools provided by an MCP server.
	KindMCP Kind = "mcp"
	// KindWeb identifies the canonical web capability.
	KindWeb Kind = "web"
	// KindTask identifies structured planning/task metadata mutations.
	KindTask Kind = "task"
	// KindAgent identifies subagent orchestration and lifecycle tools.
	KindAgent Kind = "agent"
	// KindCompute identifies deterministic local computation tools.
	KindCompute Kind = "compute"
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
	RecoveryUseDedicatedTool  RecoveryAction = "use_dedicated_tool"
)

// ToolError is an internal error with a stable model-facing classification.
type Recovery struct {
	Action    RecoveryAction  `json:"action"`
	Tool      string          `json:"tool,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

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

// Failure is the serializable failure portion of a tool result.
type Failure struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	Diagnostic string    `json:"diagnostic,omitempty"`
	Retryable  bool      `json:"retryable,omitempty"`
	Recovery   *Recovery `json:"recovery,omitempty"`
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
	arguments = json.RawMessage(strings.TrimSpace(string(arguments)))
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

// NoArgumentsSchema returns the canonical contract for tools that accept no arguments.
func NoArgumentsSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

// IsNoArgumentsSchema reports whether schema is the canonical empty-object input contract.
func IsNoArgumentsSchema(schema map[string]any) bool {
	if schema == nil || schema["type"] != "object" || schema["additionalProperties"] != false {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != 0 {
		return false
	}
	if required, ok := schema["required"]; ok {
		switch values := required.(type) {
		case []string:
			return len(values) == 0
		case []any:
			return len(values) == 0
		default:
			return false
		}
	}
	return true
}

// NormalizeArguments canonicalizes provider variations before schema validation.
// Zero-argument tools intentionally discard object-shaped metadata emitted by
// models (for example {"reason":"..."}) because no object field can carry
// semantic input for these tools. Scalars and arrays remain untouched so the
// canonical schema validator can reject genuinely malformed calls.
func NormalizeArguments(definition Definition, arguments json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(arguments))
	if IsNoArgumentsSchema(definition.InputSchema) {
		if trimmed == "" || trimmed == "null" {
			return json.RawMessage(`{}`)
		}
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(trimmed), &object) == nil && object != nil {
			return json.RawMessage(`{}`)
		}
		return append(json.RawMessage(nil), arguments...)
	}
	if len(definition.InputAliases) == 0 {
		return append(json.RawMessage(nil), arguments...)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal([]byte(trimmed), &object) != nil || object == nil {
		return append(json.RawMessage(nil), arguments...)
	}
	changed := false
	for canonical, aliases := range definition.InputAliases {
		canonicalValue, hasCanonical := object[canonical]
		for _, alias := range aliases {
			aliasValue, ok := object[alias]
			if !ok {
				continue
			}
			if hasCanonical && string(canonicalValue) != string(aliasValue) {
				return append(json.RawMessage(nil), arguments...)
			}
			if !hasCanonical {
				object[canonical] = aliasValue
				canonicalValue, hasCanonical = aliasValue, true
			}
			delete(object, alias)
			changed = true
		}
	}
	if !changed {
		return append(json.RawMessage(nil), arguments...)
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return append(json.RawMessage(nil), arguments...)
	}
	return encoded
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
	// Safety declares the resource and mutation contract enforced by the host.
	Safety SafetyContract
	// ExecutionTimeoutPolicy controls whether the service applies its generic
	// per-tool execution timeout. Orchestration tools may instead rely on a
	// stricter caller/coordinator deadline.
	ExecutionTimeoutPolicy ExecutionTimeoutPolicy
	// Evidence declares which empirical state a successful result establishes.
	// It is intentionally explicit so planning/status tools cannot accidentally
	// satisfy workspace-grounding requirements merely because they are read-only.
	Evidence EvidenceKind
	// PermissionDetailKey names the JSON argument shown to a permission
	// prompt and matched by path, command, or domain rules.
	PermissionDetailKey string
	// InputSchema is the canonical JSON-schema-like manifest exposed to callers.
	InputSchema map[string]any
	// InputAliases maps canonical fields to legacy/model compatibility aliases.
	// Aliases are accepted at ingress but are intentionally not published in InputSchema.
	InputAliases map[string][]string
	// OutputSchema optionally validates structured output returned by the tool.
	OutputSchema map[string]any
	// Semantics optionally refines mutability, safety, evidence, and risk for a
	// concrete call. This is intentionally host-only and is never published to models.
	Semantics CallSemanticsResolver
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
	if !validEvidenceKind(d.Evidence) {
		return fmt.Errorf("%w: unsupported evidence kind %q for %q", ErrInvalidCall, d.Evidence, d.Name)
	}
	return nil
}

// Pagination describes machine-readable continuation state for bounded tool output.
type Pagination struct {
	Kind         string `json:"kind"`
	NextOffset   *int64 `json:"next_offset,omitempty"`
	NextLine     *int   `json:"next_line,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

// Result is the model-facing output of a tool execution.
type Result struct {
	// CallID identifies the originating call.
	CallID string `json:"call_id"`
	// ToolName identifies the handler that produced the result.
	ToolName string `json:"tool_name"`
	// Output is the compatibility view presented to existing model/UI adapters.
	Output string `json:"output,omitempty"`
	// StructuredOutput preserves machine-readable JSON returned by tools such as MCP.
	StructuredOutput json.RawMessage `json:"structured_output,omitempty"`
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
	// Continuation is the legacy flat cursor view retained for compatibility.
	Continuation string `json:"continuation,omitempty"`
	// Pagination is the canonical machine-readable continuation state.
	Pagination *Pagination `json:"pagination,omitempty"`
	// Failure is populated when a tool call fails or is denied.
	Failure *Failure `json:"error,omitempty"`
	// SHA256 identifies the complete file content when a tool can prove a full-file snapshot.
	SHA256 string `json:"sha256,omitempty"`
	// CheckpointID identifies the pre-edit snapshot created by a mutating tool.
	CheckpointID string `json:"checkpoint_id,omitempty"`
	// MutationCoverage reports whether AffectedPaths fully describes the possible workspace mutation scope.
	MutationCoverage MutationCoverage `json:"mutation_coverage,omitempty"`
	// AffectedPaths lists workspace-relative paths successfully mutated by the tool.
	AffectedPaths []string `json:"affected_paths,omitempty"`
}

// ModelPayload returns a model-facing copy without duplicate process stream text.
// Output is already the compatibility view of stdout/stderr for process-backed
// tools; keeping all three strings in conversation history needlessly multiplies
// retained payload bytes. Stream metadata and truncation flags are preserved.
func (r Result) ModelPayload() Result {
	if r.Output != "" {
		r.Stdout = ""
		r.Stderr = ""
	}
	if r.Failure != nil {
		failure := *r.Failure
		failure.Message = compactModelFailureMessage(failure.Message)
		failure.Diagnostic = compactModelFailureMessage(failure.Diagnostic)
		r.Failure = &failure
	}
	return r
}

const maxModelFailureMessageChars = 240

func compactModelFailureMessage(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	runes := []rune(message)
	if len(runes) <= maxModelFailureMessageChars {
		return message
	}
	return strings.TrimSpace(string(runes[:maxModelFailureMessageChars-1])) + "…"
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

// ContractDiagnostic is non-sensitive metadata for diagnosing dynamic-tool schema drift.
type ContractDiagnostic struct {
	Source            string
	CatalogGeneration uint64
	SchemaFingerprint string
}

// ContractDiagnosticProvider exposes registration-time contract metadata to the dispatcher.
type ContractDiagnosticProvider interface {
	ContractDiagnostic() ContractDiagnostic
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

// BatchRegistrar atomically registers a set of handlers or leaves the registry unchanged.
type BatchRegistrar interface {
	Registrar
	RegisterBatch([]Handler) error
}

// NamespaceReplacer atomically replaces all handlers under one canonical name prefix.
type NamespaceReplacer interface {
	Registry
	ReplaceNamespace(prefix string, handlers []Handler) error
}

// DynamicRegistrar supports atomic discovery and catalog replacement.
type DynamicRegistrar interface {
	BatchRegistrar
	NamespaceReplacer
}

func validKind(kind Kind) bool {
	switch kind {
	case KindRead, KindEdit, KindBash, KindGrep, KindGit, KindMCP, KindWeb, KindTask, KindAgent, KindCompute:
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

func validEvidenceKind(kind EvidenceKind) bool {
	switch kind {
	case EvidenceNone, EvidenceWorkspace, EvidenceExternal:
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
	case KindRead, KindGrep, KindWeb, KindCompute:
		return MutabilityReadOnly
	default:
		return MutabilityMutating
	}
}

// StaticCallSemantics returns the conservative semantics implied by a tool
// definition before any call-specific refinement is applied.
func StaticCallSemantics(definition Definition) CallSemantics {
	mutability := EffectiveMutability(definition)
	effect := CommandEffectMutating
	if mutability == MutabilityReadOnly {
		effect = CommandEffectReadOnly
	}
	return CallSemantics{
		Mutability: mutability,
		Safety:     definition.Safety,
		Evidence:   definition.Evidence,
		Effect:     effect,
		Risk:       CommandRiskNormal,
		Scope:      CommandScopeUnknown,
	}
}

// EffectiveCallSemantics resolves the host-side semantics for one invocation.
// Capability facades may provide an explicit resolver; bash retains its
// command analyzer fallback for backwards compatibility.
func EffectiveCallSemantics(definition Definition, arguments json.RawMessage) CallSemantics {
	if definition.Semantics != nil {
		return definition.Semantics(arguments)
	}
	semantics := StaticCallSemantics(definition)
	if definition.Kind != KindBash {
		return semantics
	}
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		semantics.Mutability = MutabilityMutating
		semantics.Effect = CommandEffectUnknown
		semantics.Risk = CommandRiskDestructive
		return semantics
	}
	analysis := AnalyzeCommand(input.Command)
	semantics.Effect = analysis.Effect
	semantics.Risk = analysis.Risk
	semantics.Scope = analysis.Scope
	if analysis.Effect == CommandEffectReadOnly {
		semantics.Mutability = MutabilityReadOnly
	} else {
		semantics.Mutability = MutabilityMutating
	}
	if analysis.Effect == CommandEffectUnknown {
		semantics.Risk = CommandRiskDestructive
	}
	return semantics
}

// EffectiveCallEffect reports the per-call effect for grant scoping.
func EffectiveCallEffect(definition Definition, arguments json.RawMessage) CommandEffect {
	return EffectiveCallSemantics(definition, arguments).Effect
}

// EffectiveCallScope reports the proven scope of one call.
func EffectiveCallScope(definition Definition, arguments json.RawMessage) CommandScope {
	return EffectiveCallSemantics(definition, arguments).Scope
}

// EffectiveCallRisk reports proven destructive behavior for one call.
func EffectiveCallRisk(definition Definition, arguments json.RawMessage) CommandRisk {
	return EffectiveCallSemantics(definition, arguments).Risk
}

// EffectiveCallMutability reports whether one concrete call may mutate state.
func EffectiveCallMutability(definition Definition, arguments json.RawMessage) Mutability {
	return EffectiveCallSemantics(definition, arguments).Mutability
}

// EffectiveCallSafety reports the effective resource and checkpoint contract.
func EffectiveCallSafety(definition Definition, arguments json.RawMessage) SafetyContract {
	return EffectiveCallSemantics(definition, arguments).Safety
}

// EffectiveCallEvidence reports which empirical state a successful call establishes.
func EffectiveCallEvidence(definition Definition, arguments json.RawMessage) EvidenceKind {
	return EffectiveCallSemantics(definition, arguments).Evidence
}
