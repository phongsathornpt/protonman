//go:build !desktop && !desktop_gio

package gioui

import (
	"context"
	"errors"

	"github.com/phongsathornpt/protonman/internal/app"
)

func Run(_ context.Context, _ app.ACPAgents, _ app.MCPIntegrations, _ *app.DesktopPreferences) error {
	return errors.New("gio desktop requires the desktop build tag (-tags desktop)")
}

func Main() {}
