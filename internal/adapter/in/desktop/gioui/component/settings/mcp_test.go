//go:build desktop || desktop_gio

package settings

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

func mcpTestChrome() Chrome {
	button := func(gtx layout.Context, target *widget.Clickable, _ string, enabled bool, action func()) layout.Dimensions {
		if enabled && target.Clicked(gtx) && action != nil {
			action()
		}
		return target.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })
	}
	return Chrome{
		Label: func(layout.Context, string, unit.Sp, font.Weight, color.NRGBA, int) layout.Dimensions {
			return layout.Dimensions{}
		},
		RoundedSurface: func(gtx layout.Context, _ unit.Dp, _ color.NRGBA, child layout.Widget) layout.Dimensions {
			return child(gtx)
		},
		BorderSurface: func(gtx layout.Context, _ unit.Dp, _, _ color.NRGBA, _ int, child layout.Widget) layout.Dimensions {
			return child(gtx)
		},
		Inset: func(gtx layout.Context, inset layout.Inset, child layout.Widget) layout.Dimensions {
			return inset.Layout(gtx, child)
		},
		Divider:    func(layout.Context) layout.Dimensions { return layout.Dimensions{} },
		PanelTitle: func(layout.Context, string) layout.Dimensions { return layout.Dimensions{} },
		Editor: func(layout.Context, string, *widget.Editor, bool) layout.Dimensions {
			return layout.Dimensions{}
		},
		MiniIconButton: func(gtx layout.Context, target *widget.Clickable, _ string, _ color.NRGBA) layout.Dimensions {
			return target.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })
		},
		ActionIcon: func(layout.Context, uikit.Icon, unit.Dp, color.NRGBA) layout.Dimensions {
			return layout.Dimensions{}
		},
		Button:        button,
		PrimaryButton: button,
		DangerButton:  button,
		CloseButton: func(gtx layout.Context, target *widget.Clickable, close func()) layout.Dimensions {
			if target.Clicked(gtx) && close != nil {
				close()
			}
			return target.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })
		},
	}
}

func mcpTestContext(ops *op.Ops) layout.Context {
	return layout.Context{
		Ops:         ops,
		Constraints: layout.Exact(image.Pt(600, 800)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
	}
}

// The MCP panel must lay out at a usable width with a saved integration present,
// and the quick presets must fill the form rather than leaving it blank.
func TestMCPIntegrationsPanelAndPresetsLayout(t *testing.T) {
	component := New()
	chrome := mcpTestChrome()
	input := MCPInput{
		Integrations: []MCPIntegration{
			{Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}, Env: []string{"GITHUB_TOKEN"}},
		},
		Chrome: chrome,
	}

	ops := new(op.Ops)
	if dims := component.LayoutMCP(mcpTestContext(ops), input); dims.Size.X == 0 {
		t.Fatal("LayoutMCP returned 0 width")
	}

	// Open the form for a new integration, then apply each quick preset.
	component.mcp.FormVisible = true
	component.mcp.SelectedName = ""
	component.ClearMCPIntegrationEditors()

	presets := []struct {
		name       string
		button     func() *widget.Clickable
		wantName   string
		wantArgSub string
	}{
		{"github", func() *widget.Clickable { return &component.mcp.PresetGitHubButton }, "github", "server-github"},
		{"memory", func() *widget.Clickable { return &component.mcp.PresetMemoryButton }, "memory", "server-memory"},
	}
	for _, preset := range presets {
		// A Clickable only reports Clicked on the frame after the click, so the
		// first pass primes the widget and the second one applies the preset.
		preset.button().Click()
		component.LayoutMCPPresets(mcpTestContext(new(op.Ops)), true, chrome)
		component.LayoutMCPPresets(mcpTestContext(new(op.Ops)), true, chrome)
		name := component.mcp.NameEditor.Text()
		command := component.mcp.CommandEditor.Text()
		args := component.mcp.ArgsEditor.Text()
		if name != preset.wantName || command != "npx" || !strings.Contains(args, preset.wantArgSub) {
			t.Fatalf("%s preset = name:%q command:%q args:%q, want name:%q command:npx args containing %q",
				preset.name, name, command, args, preset.wantName, preset.wantArgSub)
		}
	}
}

// A busy session must disable the destructive MCP actions, otherwise a delete
// or reconnect can be issued while the agent is mid-turn.
func TestMCPPanelDisablesActionsWhileBusy(t *testing.T) {
	component := New()
	component.mcp.FormVisible = true
	component.mcp.SelectedName = "github"
	component.mcp.NameEditor.SetText("github")
	component.mcp.CommandEditor.SetText("npx")

	var removed string
	input := MCPInput{
		Integrations: []MCPIntegration{{Name: "github", Command: "npx"}},
		Updating:     true,
		Reconnecting: true,
		Chrome:       mcpTestChrome(),
		OnRemove:     func(name string) { removed = name },
	}
	component.LayoutMCP(mcpTestContext(new(op.Ops)), input)
	if removed != "" {
		t.Fatalf("OnRemove fired while the panel was busy: %q", removed)
	}
}
