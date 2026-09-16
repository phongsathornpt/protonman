package app

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/modelclient"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
)

// LanguageModelRequest is the application alias for the core model-client request.
type LanguageModelRequest = modelclient.Request

// LanguageModelFactory is the application alias for the core model-client port.
type LanguageModelFactory = modelclient.Factory

// rootMemoryBinder is the optional capability a root-session model factory
// exposes so inbound adapters can follow the active session without naming the
// durable-memory feature. It is deliberately a private structural interface:
// only the composition root wires the concrete implementation.
type rootMemoryBinder interface {
	BindSession(sessionID, workspaceKey string)
}

// BindRootMemory points the root model factory at the active session after a
// session switch. It is a no-op for factories that do not own root memory, so
// callers can invoke it unconditionally.
func BindRootMemory(factory LanguageModelFactory, sessionID, workspaceKey string) {
	binder, ok := factory.(rootMemoryBinder)
	if !ok || binder == nil {
		return
	}
	if strings.TrimSpace(workspaceKey) == "" {
		return
	}
	binder.BindSession(sessionID, workspaceKey)
}

// subagentBaseFactory is the optional capability a root-only model decorator
// exposes so callers can recover the undecorated factory underneath it.
type subagentBaseFactory interface {
	BaseFactory() LanguageModelFactory
}

// subagentLanguageModel returns the model subagent admission should inherit.
// Root-only decorators expose their base factory; when they do, the base is
// built with the same request so children never receive root-session context
// such as durable memory. Factories without the capability are returned as-is.
func subagentLanguageModel(factory LanguageModelFactory, request LanguageModelRequest, decorated port.LanguageModel) port.LanguageModel {
	carrier, ok := factory.(subagentBaseFactory)
	if !ok || carrier == nil {
		return decorated
	}
	base := carrier.BaseFactory()
	if base == nil {
		return decorated
	}
	if undecorated := base.Build(request); undecorated != nil {
		return undecorated
	}
	return decorated
}
