package tool

import (
	"encoding/json"
	"fmt"
)

// Mutability declares whether successful execution can change state that may
// invalidate prior no-progress observations. The zero value preserves legacy
// kind-based inference for third-party tools.
type Mutability string

const (
	MutabilityUnspecified Mutability = ""
	MutabilityReadOnly    Mutability = "read_only"
	MutabilityMutating    Mutability = "mutating"
)

// MutationDomain identifies which state family a tool may mutate.
type MutationDomain string

const (
	MutationDomainUnspecified     MutationDomain = ""
	MutationDomainNone            MutationDomain = "none"
	MutationDomainWorkspace       MutationDomain = "workspace"
	MutationDomainTaskState       MutationDomain = "task_state"
	MutationDomainAgentState      MutationDomain = "agent_state"
	MutationDomainWorkspacePolicy MutationDomain = "workspace_policy"
)

// MutationSafety describes how precisely a workspace mutation is scoped.
type MutationSafety string

const (
	MutationSafetyUnspecified MutationSafety = ""
	MutationSafetyNone        MutationSafety = "none"
	MutationSafetyContextual  MutationSafety = "contextual"
	MutationSafetyWholeFile   MutationSafety = "whole_file"
	MutationSafetyDynamic     MutationSafety = "dynamic"
)

// MutationCoverage reports how completely a result identifies workspace paths that may have changed.
type MutationCoverage string

const (
	MutationCoverageNone    MutationCoverage = ""
	MutationCoverageFull    MutationCoverage = "full"
	MutationCoveragePartial MutationCoverage = "partial"
	MutationCoverageUnknown MutationCoverage = "unknown"
)

// CheckpointPolicy declares whether a workspace mutation must be snapshotted.
type CheckpointPolicy string

const (
	CheckpointPolicyUnspecified CheckpointPolicy = ""
	CheckpointPolicyNone        CheckpointPolicy = "none"
	CheckpointPolicyRequired    CheckpointPolicy = "required"
	CheckpointPolicyWhenKnown   CheckpointPolicy = "when_known"
)

// BoundaryPolicy declares the resource boundary enforced by the tool.
type BoundaryPolicy string

const (
	BoundaryPolicyUnspecified    BoundaryPolicy = ""
	BoundaryPolicyNone           BoundaryPolicy = "none"
	BoundaryPolicyWorkspaceRead  BoundaryPolicy = "workspace_read"
	BoundaryPolicyWorkspaceWrite BoundaryPolicy = "workspace_write"
	BoundaryPolicyExternalRead   BoundaryPolicy = "external_read"
	BoundaryPolicySandbox        BoundaryPolicy = "sandbox"
)

// SafetyContract makes host-side safety semantics explicit instead of inferring them from tool names.
type SafetyContract struct {
	MutationDomain   MutationDomain
	MutationSafety   MutationSafety
	CheckpointPolicy CheckpointPolicy
	Boundary         BoundaryPolicy
}

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

const (
	ExecutionTimeoutServiceDefault ExecutionTimeoutPolicy = ""
	ExecutionTimeoutCallerBounded  ExecutionTimeoutPolicy = "caller"
)

// EvidenceKind declares which empirical state a successful tool result can
// establish for grounding policy. The zero value intentionally means no
// grounding evidence; evidence must be declared explicitly.
type EvidenceKind string

const (
	EvidenceNone      EvidenceKind = ""
	EvidenceWorkspace EvidenceKind = "workspace"
	EvidenceExternal  EvidenceKind = "external"
)

// CommandEffect classifies the observable state impact of a shell command.
type CommandEffect string

const (
	CommandEffectUnknown  CommandEffect = "unknown"
	CommandEffectReadOnly CommandEffect = "read_only"
	CommandEffectMutating CommandEffect = "mutating"
)

// CommandRisk classifies proven destructive behavior independently from generic mutability.
type CommandRisk string

const (
	CommandRiskNormal            CommandRisk = ""
	CommandRiskDestructive       CommandRisk = "destructive"
	CommandRiskRemoteDestructive CommandRisk = "remote_destructive"
)

// CommandScope classifies where a mutating shell command applies its external effect.
type CommandScope string

const (
	CommandScopeUnknown    CommandScope = ""
	CommandScopeLocal      CommandScope = "local"
	CommandScopeRemote     CommandScope = "remote"
	CommandScopePublish    CommandScope = "publish"
	CommandScopeDeployment CommandScope = "deployment"
)

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
