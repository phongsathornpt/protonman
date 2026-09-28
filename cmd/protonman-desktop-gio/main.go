//go:build desktop || desktop_gio

package main

import (
	"context"
	"log"
	"os"

	desktopgio "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/app"
)

func main() {
	agents := app.NewACPAgents(config.NewUserACPAgentsStore(""))
	integrations := app.NewMCPIntegrations(config.NewUserMCPIntegrationsStore(""))
	preferences := app.NewDesktopPreferences(config.NewUserDesktopPreferencesStore(""))
	go func() {
		if err := desktopgio.Run(context.Background(), agents, integrations, preferences); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	desktopgio.Main()
}
