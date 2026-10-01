//go:build desktop || desktop_gio

package shell

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// Bindings is the shell's complete set of application actions. The shell renders
// and emits intent; it never reaches into the controller itself. Grouping the
// callbacks in one exported struct keeps that boundary explicit and makes an
// unbound action a compile error at wiring time rather than a nil call at click
// time.
type Bindings struct {
	SetTheme                 func(string)
	SelectSession            func(string, string)
	SelectProject            func(string)
	NewSession               func()
	DeleteSession            func(string, string)
	RenameSession            func(string, string, string)
	TogglePinSession         func(string, string)
	ToggleSkill              func(string, string)
	SetFilterMode            func(string)
	SendPrompt               func(controller.ExpandedPrompt)
	CancelPrompt             func()
	ResolvePermission        func(string, string)
	ResolveQuestion          func(string, desktopstate.QuestionResponse)
	SetRuntimeModel          func(string, string)
	SetRuntimeReasoning      func(string)
	SetRuntimeLow            func(string)
	SetRuntimePermissionMode func(string)
	RefreshRuntime           func()
	SaveMCPIntegration       func(string, string, string, string)
	RemoveMCPIntegration     func(string)
	ReconnectMCP             func()
	SelectAgent              func(string)
	SaveAgentProfile         func(string, string, string, string, string, string)
	RemoveAgentProfile       func(string)
	ScanDeviceAgents         func()
	SaveProvider             func(controller.ProvidersSaveParams, func(error))
	DeleteProvider           func(string, func(error))
	FetchProviderModels      func(string, string, string, string, func([]string, error))
	RefreshProviders         func()
}

// withDefaults returns a copy of the bindings with every unset action replaced
// by a no-op, so a partially wired shell renders and handles input without
// panicking. It is the caller's job to supply the actions it actually needs.
func (b Bindings) withDefaults() Bindings {
	if b.SetTheme == nil {
		b.SetTheme = func(string) {}
	}
	if b.SelectSession == nil {
		b.SelectSession = func(string, string) {}
	}
	if b.SelectProject == nil {
		b.SelectProject = func(string) {}
	}
	if b.NewSession == nil {
		b.NewSession = func() {}
	}
	if b.DeleteSession == nil {
		b.DeleteSession = func(string, string) {}
	}
	if b.RenameSession == nil {
		b.RenameSession = func(string, string, string) {}
	}
	if b.TogglePinSession == nil {
		b.TogglePinSession = func(string, string) {}
	}
	if b.ToggleSkill == nil {
		b.ToggleSkill = func(string, string) {}
	}
	if b.SetFilterMode == nil {
		b.SetFilterMode = func(string) {}
	}
	if b.SendPrompt == nil {
		b.SendPrompt = func(controller.ExpandedPrompt) {}
	}
	if b.CancelPrompt == nil {
		b.CancelPrompt = func() {}
	}
	if b.ResolvePermission == nil {
		b.ResolvePermission = func(string, string) {}
	}
	if b.ResolveQuestion == nil {
		b.ResolveQuestion = func(string, desktopstate.QuestionResponse) {}
	}
	if b.SetRuntimeModel == nil {
		b.SetRuntimeModel = func(string, string) {}
	}
	if b.SetRuntimeReasoning == nil {
		b.SetRuntimeReasoning = func(string) {}
	}
	if b.SetRuntimeLow == nil {
		b.SetRuntimeLow = func(string) {}
	}
	if b.SetRuntimePermissionMode == nil {
		b.SetRuntimePermissionMode = func(string) {}
	}
	if b.RefreshRuntime == nil {
		b.RefreshRuntime = func() {}
	}
	if b.SaveMCPIntegration == nil {
		b.SaveMCPIntegration = func(string, string, string, string) {}
	}
	if b.RemoveMCPIntegration == nil {
		b.RemoveMCPIntegration = func(string) {}
	}
	if b.ReconnectMCP == nil {
		b.ReconnectMCP = func() {}
	}
	if b.SelectAgent == nil {
		b.SelectAgent = func(string) {}
	}
	if b.SaveAgentProfile == nil {
		b.SaveAgentProfile = func(string, string, string, string, string, string) {}
	}
	if b.RemoveAgentProfile == nil {
		b.RemoveAgentProfile = func(string) {}
	}
	if b.ScanDeviceAgents == nil {
		b.ScanDeviceAgents = func() {}
	}
	if b.SaveProvider == nil {
		b.SaveProvider = func(controller.ProvidersSaveParams, func(error)) {}
	}
	if b.DeleteProvider == nil {
		b.DeleteProvider = func(string, func(error)) {}
	}
	if b.FetchProviderModels == nil {
		b.FetchProviderModels = func(string, string, string, string, func([]string, error)) {}
	}
	if b.RefreshProviders == nil {
		b.RefreshProviders = func() {}
	}
	return b
}
