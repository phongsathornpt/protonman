//go:build desktop

package main

import (
	"context"
	"embed"
	"log"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend/dist
var frontendAssets embed.FS

func main() {
	controller := desktop.NewController()
	defer controller.Close()

	app := &DesktopApp{controller: controller}
	if err := wails.Run(&options.App{
		Title:  "Protonman",
		Width:  1220,
		Height: 780,
		AssetServer: &assetserver.Options{
			Assets: frontendAssets,
		},
		Bind: []interface{}{
			app,
		},
	}); err != nil {
		log.Fatal(err)
	}
}

type DesktopApp struct {
	controller *desktop.Controller
}

func (a *DesktopApp) Snapshot() desktop.Snapshot {
	return a.controller.Snapshot(context.Background())
}

func (a *DesktopApp) SetStatus(status string) {
	a.controller.SetStatus(status)
}
