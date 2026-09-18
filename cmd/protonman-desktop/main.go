//go:build desktop

package main

import (
	"context"
	"embed"
	"log"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed frontend/dist
var frontendAssets embed.FS

func main() {
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
		OnShutdown: controller.Close,
	})

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
}

func (a *DesktopService) Snapshot() desktop.Snapshot {
	return a.controller.Snapshot(context.Background())
}

func (a *DesktopService) SetStatus(status string) {
	a.controller.SetStatus(status)
}
