//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"os"
	"strings"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/unit"

	application "github.com/phongsathornpt/protonman/internal/app"
)

func Run(ctx context.Context, agents application.ACPAgents, mcpIntegrations application.MCPIntegrations) error {
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

	controller := newController(windowContext, window.Invalidate, agents, mcpIntegrations)
	defer controller.close()
	view := newShell(newTheme(normalizedThemeMode(os.Getenv("PROTONMAN_GIO_THEME"))))
	view.onSelectSession = controller.selectSession
	view.onSelectProject = controller.selectProject
	view.onNewSession = controller.newSession
	view.onSendPrompt = controller.sendPrompt
	view.onCancelPrompt = controller.cancelPrompt
	view.onResolvePermission = controller.resolvePermission
	view.onSetRuntimeModel = controller.setRuntimeModel
	view.onSetRuntimeReasoning = controller.setRuntimeReasoning
	view.onSetRuntimeLow = controller.setRuntimeLowConcurrency
	view.onSaveMCPIntegration = controller.saveMCPIntegration
	view.onRemoveMCPIntegration = controller.removeMCPIntegration
	view.onReconnectMCP = controller.reconnectMCP
	view.onSelectAgent = controller.selectAgent
	view.onSaveAgentProfile = controller.saveAgentProfile
	view.onRemoveAgentProfile = controller.removeAgentProfile
	var operations op.Ops
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.FrameEvent:
			gtx := app.NewContext(&operations, event)
			view.layout(gtx, controller.snapshot())
			event.Frame(gtx.Ops)
		}
	}
}

func normalizedThemeMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "dark" {
		return "dark"
	}
	return "light"
}
