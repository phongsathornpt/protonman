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

// themePollInterval bounds how often the event loop probes the OS appearance.
// The probe shells out on every platform, so keeping it off the render path and
// rate-limited matters more than reacting instantly.
const themePollInterval = 10 * time.Second

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
	if preferences != nil {
		if preferenceTheme := preferences.Snapshot().Theme; preferenceTheme != "" {
			configuredTheme = uikit.NormalizeThemeMode(preferenceTheme)
		}
	}
	systemDark := uikit.IsSystemDarkMode()
	currentResolved := uikit.ResolveThemeMode(configuredTheme, systemDark)
	lastThemeCheck := time.Now()

	// applyTheme must only run on the Gio event-loop goroutine: it mutates both
	// the shell and this function's theme bookkeeping. The poller below merely
	// requests a frame; the actual check happens in FrameEvent.
	var view *shell.Shell
	applyTheme := func(newConfigured string) {
		configuredTheme = uikit.NormalizeThemeMode(newConfigured)
		systemDark = uikit.IsSystemDarkMode()
		view.SetSystemDark(systemDark)
		resolved := uikit.ResolveThemeMode(configuredTheme, systemDark)
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
		DeleteSession:            func(agentID, sessionID string) { ctrl.DeleteSessionAsync(sessionID, agentID) },
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
		RefreshProviders:         ctrl.RefreshProvidersAsync,
	})

	view.SetSystemDark(systemDark)

	// A "system" theme follows the OS appearance. The poller only requests a
	// repaint; FrameEvent performs the (rate-limited) OS check on the event-loop
	// goroutine so the shell is never mutated concurrently with Layout.
	go func() {
		ticker := time.NewTicker(themePollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-windowContext.Done():
				return
			case <-ticker.C:
				window.Invalidate()
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
				lastThemeCheck = time.Now()
				applyTheme("system")
			}
		case app.FrameEvent:
			if configuredTheme == "system" && time.Since(lastThemeCheck) >= themePollInterval {
				lastThemeCheck = time.Now()
				applyTheme("system")
			}
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
