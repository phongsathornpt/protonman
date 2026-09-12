package tool

import (
	"context"
	"encoding/json"
)

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
