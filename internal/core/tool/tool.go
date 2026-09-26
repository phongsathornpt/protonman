// Package tool defines the provider-neutral contract for executable tools.
package tool

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Kind classifies a tool for permission policy matching.
type Kind string

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
		object := map[string]json.RawMessage{}
		if json.Unmarshal([]byte(trimmed), &object) == nil && object != nil {
			return json.RawMessage(`{}`)
		}
		return append(json.RawMessage(nil), arguments...)
	}
	aliases := len(definition.InputAliases) > 0
	enums := schemaEnumFields(definition.InputSchema)
	integers := schemaIntegerFields(definition.InputSchema)
	if !aliases && len(enums) == 0 && len(integers) == 0 {
		return append(json.RawMessage(nil), arguments...)
	}
	object := map[string]json.RawMessage{}
	if json.Unmarshal([]byte(trimmed), &object) != nil || object == nil {
		return append(json.RawMessage(nil), arguments...)
	}
	changed := false
	if aliases {
		changed = applyInputAliases(object, definition.InputAliases) || changed
	}
	if len(enums) > 0 {
		changed = foldEnumFields(object, enums) || changed
	}
	if len(integers) > 0 {
		changed = canonicalIntegerFields(object, integers) || changed
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

// canonicalIntegerFields rewrites integral JSON numbers such as 3.0 into the
// integer form 3. JSON Schema treats 3.0 as an integer, so the compiled input
// validator accepts it, but encoding/json cannot unmarshal it into an int and
// the handler fails after validation has already passed. Non-integral numbers
// keep their spelling so the schema reports the type mismatch.
func canonicalIntegerFields(object map[string]json.RawMessage, fields map[string]struct{}) bool {
	changed := false
	for field := range fields {
		raw, ok := object[field]
		if !ok {
			continue
		}
		canonical, ok := canonicalIntegralNumber(raw)
		if !ok {
			continue
		}
		object[field] = canonical
		changed = true
	}
	return changed
}

// canonicalIntegralNumber converts a JSON number with a zero fraction part into
// its integer spelling. JSON Schema treats 3.0 and 1e2 as integers, so the
// compiled input validator accepts them, but encoding/json cannot unmarshal
// either into an int and the handler then fails after validation has passed.
// Values outside the int64 range and non-integral values keep their spelling so
// the schema reports the mismatch.
func canonicalIntegralNumber(raw json.RawMessage) (json.RawMessage, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || !strings.ContainsAny(trimmed, ".eE") {
		return nil, false
	}
	if !json.Valid(json.RawMessage(trimmed)) {
		return nil, false
	}
	if strings.ContainsAny(trimmed, "123456789") {
		approximation, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || approximation == 0 {
			return nil, false
		}
	}
	value, ok := new(big.Rat).SetString(trimmed)
	if !ok || !value.IsInt() || !value.Num().IsInt64() {
		return nil, false
	}
	encoded, err := json.Marshal(value.Num().Int64())
	if err != nil || string(encoded) == trimmed {
		return nil, false
	}
	return encoded, true
}

// schemaIntegerFields collects the names of root-level integer fields so
// integral-number spellings can be canonicalized before schema validation.
func schemaIntegerFields(schema map[string]any) map[string]struct{} {
	if schema == nil {
		return nil
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	fields := map[string]struct{}{}
	for name, raw := range properties {
		field, ok := raw.(map[string]any)
		if !ok || field["type"] != "integer" {
			continue
		}
		fields[name] = struct{}{}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// applyInputAliases renames compatibility aliases onto their canonical fields.
// Conflicting aliases leave the payload untouched so the schema rejects it
// rather than the host silently choosing one of two different values.
func applyInputAliases(object map[string]json.RawMessage, aliases map[string][]string) bool {
	changed := false
	for canonical, names := range aliases {
		canonicalValue, hasCanonical := object[canonical]
		for _, alias := range names {
			aliasValue, ok := object[alias]
			if !ok {
				continue
			}
			if hasCanonical && string(canonicalValue) != string(aliasValue) {
				return false
			}
			if !hasCanonical {
				object[canonical] = aliasValue
				canonicalValue, hasCanonical = aliasValue, true
			}
			delete(object, alias)
			changed = true
		}
	}
	return changed
}

// foldEnumFields lowercases enumerated string fields so the published enum is
// never stricter than the handler that dispatches on the value. Handlers match
// these values case-insensitively, so folding is behavior-preserving. A value is
// only rewritten when it matches an enum entry case-insensitively; anything else
// keeps its original spelling for the schema to reject.
func foldEnumFields(object map[string]json.RawMessage, fields map[string][]string) bool {
	changed := false
	for field, values := range fields {
		raw, ok := object[field]
		if !ok {
			continue
		}
		var text string
		if json.Unmarshal(raw, &text) != nil {
			continue
		}
		canonical, ok := matchEnumValue(values, text)
		if !ok {
			continue
		}
		encoded, err := json.Marshal(canonical)
		if err != nil || string(encoded) == string(raw) {
			continue
		}
		object[field] = encoded
		changed = true
	}
	return changed
}

// matchEnumValue returns the canonical enum entry equal to value ignoring case.
func matchEnumValue(values []string, value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	for _, candidate := range values {
		if strings.EqualFold(strings.TrimSpace(candidate), trimmed) {
			return candidate, true
		}
	}
	return "", false
}

// schemaEnumFields collects root-level string enum fields published by a tool
// input schema. Nested enums (for example inside an array item schema) are left
// to the schema because their location is call-specific.
func schemaEnumFields(schema map[string]any) map[string][]string {
	if schema == nil {
		return nil
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	fields := map[string][]string{}
	for name, raw := range properties {
		field, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typeName, _ := field["type"].(string)
		if typeName != "" && typeName != "string" {
			continue
		}
		values, ok := enumStrings(field["enum"])
		if !ok || len(values) == 0 {
			continue
		}
		fields[name] = values
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// enumStrings extracts enum entries that are plain strings and lowercase-simple.
// Enums with non-string or non-lowercase entries are skipped so unexpected
// vocabulary is never silently case-folded.
func enumStrings(raw any) ([]string, bool) {
	var entries []any
	switch values := raw.(type) {
	case []any:
		entries = values
	case []string:
		entries = make([]any, 0, len(values))
		for _, value := range values {
			entries = append(entries, value)
		}
	default:
		return nil, false
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		text, ok := entry.(string)
		if !ok || strings.TrimSpace(text) == "" || text != strings.ToLower(text) {
			return nil, false
		}
		out = append(out, text)
	}
	return out, true
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
	// ProviderOptions carries adapter-specific publication metadata. The
	// canonical schema and host validation remain provider-neutral; adapters
	// decide which options are meaningful for their wire protocol.
	ProviderOptions map[string]json.RawMessage
	// OutputSchema optionally validates structured output returned by the tool.
	OutputSchema map[string]any
	// Semantics optionally refines mutability, safety, evidence, and risk for a
	// concrete call. This is intentionally host-only and is never published to models.
	Semantics CallSemanticsResolver
}

// ArgumentNormalizer is an optional handler capability that canonicalizes
// model-originated arguments beyond what the published JSON schema can express,
// for example a numeric string where the schema requires an integer. It runs
// after schema-driven alias normalization and before schema validation, so a
// handled wire variant reaches the schema in its canonical form instead of
// failing validation with a message the model cannot act on.
//
// Implementations must return nil when no rewrite is needed; the caller then
// keeps the original bytes verbatim. They must not repair arguments that are
// genuinely invalid, because the schema is the authority for what a call means.
type ArgumentNormalizer interface {
	NormalizeArguments(arguments json.RawMessage) json.RawMessage
}

// NormalizeArgumentsForHandler applies schema-driven aliasing and then any
// canonicalization offered by the handler for this call.
func NormalizeArgumentsForHandler(handler Handler, definition Definition, arguments json.RawMessage) json.RawMessage {
	normalized := NormalizeArguments(definition, arguments)
	normalizer, ok := handler.(ArgumentNormalizer)
	if !ok {
		return normalized
	}
	result := normalizer.NormalizeArguments(normalized)
	if len(result) == 0 {
		return normalized
	}
	return result
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

// ImageAttachment represents an optional multimodal image produced by a tool.
type ImageAttachment struct {
	MIMEType string `json:"mime_type"`
	Data     string `json:"data,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
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
	// NextLine is the continuation line offset for line-oriented pageable tools.
	NextLine *int `json:"next_line,omitempty"`
	// Continuation is an opaque continuation token for stream-like tools.
	Continuation string `json:"continuation,omitempty"`
	// Pagination is the canonical machine-readable continuation state.
	Pagination *Pagination `json:"pagination,omitempty"`
	// Image is populated when a tool produces an image for multimodal vision models.
	Image *ImageAttachment `json:"image,omitempty"`
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

// NormalizeOutput ensures Output is non-empty when StructuredOutput or streams exist.
func (r *Result) NormalizeOutput() {
	if r.Output != "" {
		return
	}
	if len(r.StructuredOutput) > 0 && string(r.StructuredOutput) != "{}" {
		r.Output = string(r.StructuredOutput)
		return
	}
	if r.Stdout != "" || r.Stderr != "" {
		switch {
		case r.Stdout != "" && r.Stderr != "":
			r.Output = r.Stdout + "\n" + r.Stderr
		case r.Stdout != "":
			r.Output = r.Stdout
		default:
			r.Output = r.Stderr
		}
	}
}

// ModelPayload returns a compacted view of Result suitable for model consumption.
func (r Result) ModelPayload() Result {
	if r.Output != "" {
		r.Stdout = ""
		r.Stderr = ""
	}
	if r.Image != nil && r.Image.Data != "" {
		img := *r.Image
		img.Data = ""
		r.Image = &img
	}
	if r.Failure != nil {
		failure := *r.Failure
		failure.Message = compactModelFailureMessage(failure.Message)
		failure.Diagnostic = compactModelFailureMessage(failure.Diagnostic)
		r.Failure = &failure
	}
	return r
}

// maxModelFailureMessageChars bounds failure text delivered to a model. It is
// deliberately generous: schema-validation diagnostics name the offending JSON
// pointer and the mismatch, and truncating that clause removes the only
// actionable part of the message.
const maxModelFailureMessageChars = 600

func compactModelFailureMessage(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	runes := []rune(message)
	if len(runes) <= maxModelFailureMessageChars {
		return message
	}
	return strings.TrimSpace(string(runes[:maxModelFailureMessageChars-1])) + "…"
}
