//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"os"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/unit"

	application "github.com/phongsathornpt/protonman/internal/app"
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

	controller := newController(windowContext, window.Invalidate, agents, mcpIntegrations, preferences)
	defer controller.close()
	configuredTheme := normalizedThemeMode(os.Getenv("PROTONMAN_GIO_THEME"))
	if preferences != nil && preferences.Snapshot().Theme != "" {
		configuredTheme = normalizedThemeMode(preferences.Snapshot().Theme)
	}
	currentResolved := resolveThemeMode(configuredTheme, isSystemDarkMode())
	view := newShell(newTheme(currentResolved))

	updateTheme := func(newConfigured string) {
		configuredTheme = normalizedThemeMode(newConfigured)
		resolved := resolveThemeMode(configuredTheme, isSystemDarkMode())
		if resolved != currentResolved {
			currentResolved = resolved
			view.theme = newTheme(resolved)
			window.Invalidate()
		}
	}

	view.onSetTheme = func(themeMode string) {
		controller.setTheme(themeMode)
		updateTheme(themeMode)
	}

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-windowContext.Done():
				return
			case <-ticker.C:
				if configuredTheme == "system" {
					updateTheme("system")
				}
			}
		}
	}()
	view.onSelectSession = controller.selectSession
	view.onSelectProject = controller.selectProject
	view.onNewSession = controller.newSession
	view.onDeleteSession = controller.deleteSession
	view.onRenameSession = controller.renameSession
	view.onTogglePinSession = controller.togglePinSession
	view.onToggleSkill = controller.toggleSkill
	view.onSetFilterMode = controller.setFilterMode
	view.onSendPrompt = controller.sendExpandedPrompt
	view.onCancelPrompt = controller.cancelPrompt
	view.onResolvePermission = controller.resolvePermission
	view.onResolveQuestion = controller.resolveQuestion
	view.onSetRuntimeModel = controller.setRuntimeModel
	view.onSetRuntimeReasoning = controller.setRuntimeReasoning
	view.onSetRuntimeLow = controller.setRuntimeLowConcurrency
	view.onSetRuntimePermissionMode = controller.setRuntimePermissionMode
	view.onRefreshRuntime = func() {
		controller.refreshActiveSession(true)
	}
	view.onSaveMCPIntegration = controller.saveMCPIntegration
	view.onRemoveMCPIntegration = controller.removeMCPIntegration
	view.onReconnectMCP = controller.reconnectMCP
	view.onSelectAgent = controller.selectAgent
	view.onSaveAgentProfile = controller.saveAgentProfile
	view.onRemoveAgentProfile = controller.removeAgentProfile
	view.onScanDeviceAgents = controller.scanDeviceAgents
	view.onSaveProvider = controller.saveProvider
	view.onDeleteProvider = controller.deleteProvider
	view.onFetchProviderModels = controller.fetchProviderModels
	view.onRefreshProviders = controller.refreshProviders
	var operations op.Ops
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.ConfigEvent:
			if configuredTheme == "system" {
				updateTheme("system")
			}
		case app.FrameEvent:
			gtx := app.NewContext(&operations, event)
			view.layout(gtx, controller.snapshot())
			event.Frame(gtx.Ops)
		}
	}
}

// Main hands over control of the main thread to the platform event loop.
// On macOS and mobile platforms, it must be called from the main goroutine.
func Main() {
	app.Main()
}

func normalizedThemeMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "system", "auto", "device", "":
		return "system"
	case "dark", "light", "slate-dark", "slate-light":
		return value
	}
	if strings.Contains(value, "dark") {
		return "dark"
	}
	if strings.Contains(value, "light") {
		return "light"
	}
	return "system"
}
