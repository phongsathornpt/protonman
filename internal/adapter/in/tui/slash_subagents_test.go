package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestSlashSubagentsToggle(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read_file", Kind: tool.KindRead, Description: "read"}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	m := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	m.agents = app.NewAgents(coord)

	m.executeCommand("/subagents off")
	if m.subagentsEnabled || coord.Enabled() {
		t.Fatal("/subagents off did not disable runtime capability")
	}
	if !strings.Contains(m.viewport.View(), "Subagents disabled") {
		t.Fatalf("missing disable confirmation: %q", m.viewport.View())
	}

	m.executeCommand("/subagents on")
	if !m.subagentsEnabled || !coord.Enabled() {
		t.Fatal("/subagents on did not enable runtime capability")
	}

	m.executeCommand("/subagents nope")
	if !strings.Contains(m.viewport.View(), "subagents must be on or off") {
		t.Fatalf("missing invalid toggle error: %q", m.viewport.View())
	}
}
