//go:build desktop

package main

import (
	"context"
	"embed"
	"log"
	"os"
	"sync"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed frontend/dist
var frontendAssets embed.FS

func runDesktop() {
	controller := desktop.NewController()

	service := &DesktopService{controller: controller}
	app := application.New(application.Options{
		Name:        "Protonman",
		Description: "Protonman desktop client",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(frontendAssets),
		},
		OnShutdown: func() {
			service.close()
			controller.Close()
		},
	})
	service.bind(app)

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "main",
		Title:  "Protonman",
		Width:  1220,
		Height: 780,
		URL:    "/",
	})
	window.Show()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

type DesktopService struct {
	controller *desktop.Controller
	mu         sync.Mutex
	unsub      func()
	cancel     context.CancelFunc
	app        *application.App
}

func (a *DesktopService) Snapshot() desktop.Snapshot {
	return a.controller.Snapshot(context.Background())
}

func (a *DesktopService) Bootstrap() (desktop.Snapshot, error) {
	workspace, err := os.Getwd()
	if err != nil {
		workspace = "."
	}
	return a.controller.Bootstrap("", workspace)
}

func (a *DesktopService) NewSession(workspace string, additionalDirectories []string, mcpServers []desktop.MCPServerView) (desktop.Snapshot, error) {
	return a.controller.NewSession(context.Background(), workspace, additionalDirectories, mcpServers)
}

func (a *DesktopService) SelectSession(sessionID string) (desktop.Snapshot, error) {
	return a.controller.SelectSession(context.Background(), sessionID)
}

func (a *DesktopService) SetMode(sessionID, modeID string) (desktop.Snapshot, error) {
	return a.controller.SetMode(context.Background(), sessionID, modeID)
}

func (a *DesktopService) SetReasoning(sessionID, reasoning string) (desktop.Snapshot, error) {
	return a.controller.SetReasoning(context.Background(), sessionID, reasoning)
}

func (a *DesktopService) SetModel(sessionID, modelID string) (desktop.Snapshot, error) {
	return a.controller.SetModel(context.Background(), sessionID, modelID)
}

func (a *DesktopService) SetProvider(sessionID, provider string) (desktop.Snapshot, error) {
	return a.controller.SetProvider(context.Background(), sessionID, provider)
}

func (a *DesktopService) SetLowConcurrency(sessionID, setting string) (desktop.Snapshot, error) {
	return a.controller.SetLowConcurrency(context.Background(), sessionID, setting)
}

func (a *DesktopService) ForgetMemory(sessionID, scope, memoryID string) (desktop.Snapshot, error) {
	return a.controller.ForgetMemory(context.Background(), sessionID, scope, memoryID)
}

func (a *DesktopService) UpdateTodoStatus(sessionID string, revision uint64, itemID, status string) (desktop.Snapshot, error) {
	return a.controller.UpdateTodoStatus(context.Background(), sessionID, revision, itemID, status)
}

func (a *DesktopService) PatchTodo(sessionID string, revision uint64, operation desktop.TodoOperationView) (desktop.Snapshot, error) {
	return a.controller.PatchTodo(context.Background(), sessionID, revision, operation)
}

func (a *DesktopService) CloseSession(sessionID string) (desktop.Snapshot, error) {
	return a.controller.CloseSession(context.Background(), sessionID)
}

func (a *DesktopService) DeleteSession(sessionID string) (desktop.Snapshot, error) {
	return a.controller.DeleteSession(context.Background(), sessionID)
}

func (a *DesktopService) SendPrompt(sessionID, text string) (desktop.Snapshot, error) {
	return a.controller.SendPrompt(context.Background(), sessionID, text)
}

func (a *DesktopService) CancelPrompt(sessionID string) (desktop.Snapshot, error) {
	return a.controller.CancelPrompt(context.Background(), sessionID)
}

func (a *DesktopService) ResolvePermission(requestID, optionID string) (desktop.Snapshot, error) {
	return a.controller.ResolvePermission(requestID, optionID)
}

func (a *DesktopService) bind(app *application.App) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.app = app
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	updates, unsubscribe := a.controller.Subscribe(ctx)
	a.unsub = unsubscribe
	go func() {
		for snapshot := range updates {
			app.Event.Emit(desktop.EventSnapshot, snapshot)
		}
	}()
}

func (a *DesktopService) close() {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	if a.unsub != nil {
		a.unsub()
	}
	a.cancel = nil
	a.unsub = nil
	a.app = nil
	a.mu.Unlock()
}
