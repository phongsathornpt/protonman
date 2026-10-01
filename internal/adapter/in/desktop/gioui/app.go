//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/unit"

	application "github.com/phongsathornpt/protonman/internal/app"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/shell"
)

func Run(ctx context.Context, agents application.ACPAgents, mcpIntegrations application.MCPIntegrations, preferences *application.DesktopPreferences) error {
	windowContext, cancel := context.WithCancel(ctx)
	defer cancel()

	var window app.Window
	window.Option(
		app.Title("Protonman"),
		app.Size(unit.Dp(1180), unit.Dp(760)),
		app.MinSize(unit.Dp(760), unit.Dp(600)),
	)
	go func() {
		<-windowContext.Done()
		window.Perform(system.ActionClose)
	}()

	ctrl := controller.New(windowContext, window.Invalidate, agents, mcpIntegrations, preferences)
	defer ctrl.Close()

	configuredTheme := uikit.NormalizeThemeMode(os.Getenv("PROTONMAN_GIO_THEME"))
	if preferences != nil && preferences.Snapshot().Theme != "" {
		configuredTheme = uikit.NormalizeThemeMode(preferences.Snapshot().Theme)
	}
	currentResolved := uikit.ResolveThemeMode(configuredTheme, uikit.IsSystemDarkMode())

	// applyTheme resolves the configured mode and repaints when it actually
	// changed, so a repeated "system" poll does not thrash the window. view is
	// assigned immediately after construction, before any binding can fire.
	var view *shell.Shell
	applyTheme := func(newConfigured string) {
		configuredTheme = uikit.NormalizeThemeMode(newConfigured)
		resolved := uikit.ResolveThemeMode(configuredTheme, uikit.IsSystemDarkMode())
		if resolved != currentResolved {
			currentResolved = resolved
			view.SetTheme(shell.NewTheme(resolved))
			window.Invalidate()
		}
	}

	// The shell owns presentation only: every action below routes into the
	// controller, and the controller never reaches back into the shell.
	view = shell.New(shell.NewTheme(currentResolved), shell.Bindings{
		SetTheme: func(themeMode string) {
			ctrl.SetTheme(themeMode)
			applyTheme(themeMode)
		},
		SelectSession:            ctrl.SelectSessionForAgent,
		SelectProject:            ctrl.SelectProject,
		NewSession:               ctrl.NewSession,
		DeleteSession:            func(agentID, sessionID string) { ctrl.DeleteSession(sessionID, agentID) },
		RenameSession:            func(agentID, sessionID, title string) { ctrl.RenameSession(sessionID, title, agentID) },
		TogglePinSession:         func(agentID, sessionID string) { ctrl.TogglePinSession(sessionID, agentID) },
		ToggleSkill:              ctrl.ToggleSkill,
		SetFilterMode:            ctrl.SetFilterMode,
		SendPrompt:               ctrl.SendExpandedPrompt,
		CancelPrompt:             ctrl.CancelPrompt,
		ResolvePermission:        ctrl.ResolvePermission,
		ResolveQuestion:          ctrl.ResolveQuestion,
		SetRuntimeModel:          ctrl.SetRuntimeModel,
		SetRuntimeReasoning:      ctrl.SetRuntimeReasoning,
		SetRuntimeLow:            ctrl.SetRuntimeLowConcurrency,
		SetRuntimePermissionMode: ctrl.SetRuntimePermissionMode,
		RefreshRuntime:           func() { ctrl.RefreshActiveSession(true) },
		SaveMCPIntegration:       ctrl.SaveMCPIntegration,
		RemoveMCPIntegration:     ctrl.RemoveMCPIntegration,
		ReconnectMCP:             ctrl.ReconnectMCP,
		SelectAgent:              ctrl.SelectAgent,
		SaveAgentProfile:         ctrl.SaveAgentProfile,
		RemoveAgentProfile:       ctrl.RemoveAgentProfile,
		ScanDeviceAgents:         ctrl.ScanDeviceAgents,
		SaveProvider:             ctrl.SaveProvider,
		DeleteProvider:           ctrl.DeleteProvider,
		FetchProviderModels:      ctrl.FetchProviderModels,
		RefreshProviders:         ctrl.RefreshProviders,
	})

	// A "system" theme follows the OS appearance, so poll for changes the way the
	// ACP/TUI frontends do rather than only reacting to window config events.
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-windowContext.Done():
				return
			case <-ticker.C:
				if configuredTheme == "system" {
					applyTheme("system")
				}
			}
		}
	}()
	var operations op.Ops
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.ConfigEvent:
			if configuredTheme == "system" {
				applyTheme("system")
			}
		case app.FrameEvent:
			gtx := app.NewContext(&operations, event)
			view.Layout(gtx, ctrl.Snapshot())
			event.Frame(gtx.Ops)
		}
	}
}

// Main hands over control of the main thread to the platform event loop.
// On macOS and mobile platforms, it must be called from the main goroutine.
func Main() {
	app.Main()
}
