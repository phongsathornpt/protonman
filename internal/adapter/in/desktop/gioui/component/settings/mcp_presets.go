//go:build desktop || desktop_gio

package settings

import (
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
)

// LayoutMCPPresets owns the quick-preset interactions for MCP integration forms.
func (c *Component) LayoutMCPPresets(gtx layout.Context, enabled bool, chrome Chrome) layout.Dimensions {
	mcp := &c.mcp
	if mcp.PresetGitHubButton.Clicked(gtx) {
		mcp.NameEditor.SetText("github")
		mcp.CommandEditor.SetText("npx")
		mcp.ArgsEditor.SetText("-y @modelcontextprotocol/server-github")
		mcp.EnvEditor.SetText("GITHUB_PERSONAL_ACCESS_TOKEN")
	}
	if mcp.PresetMemoryButton.Clicked(gtx) {
		mcp.NameEditor.SetText("memory")
		mcp.CommandEditor.SetText("npx")
		mcp.ArgsEditor.SetText("-y @modelcontextprotocol/server-memory")
		mcp.EnvEditor.SetText("")
	}
	if mcp.PresetFilesystemButton.Clicked(gtx) {
		mcp.NameEditor.SetText("filesystem")
		mcp.CommandEditor.SetText("npx")
		mcp.ArgsEditor.SetText("-y @modelcontextprotocol/server-filesystem .")
		mcp.EnvEditor.SetText("")
	}
	if mcp.PresetFetchButton.Clicked(gtx) {
		mcp.NameEditor.SetText("fetch")
		mcp.CommandEditor.SetText("uvx")
		mcp.ArgsEditor.SetText("mcp-server-fetch")
		mcp.EnvEditor.SetText("")
	}
	if mcp.PresetCustomButton.Clicked(gtx) {
		mcp.NameEditor.SetText("")
		mcp.CommandEditor.SetText("")
		mcp.ArgsEditor.SetText("")
		mcp.EnvEditor.SetText("")
		mcp.ConfirmDelete = false
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "QUICK PRESETS", unit.Sp(11), font.Bold, chrome.Colors.OnSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.LayoutPresetChip(gtx, &mcp.PresetGitHubButton, "GitHub", enabled, chrome)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
							return c.LayoutPresetChip(gtx, &mcp.PresetMemoryButton, "Memory", enabled, chrome)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
							return c.LayoutPresetChip(gtx, &mcp.PresetFilesystemButton, "Filesystem", enabled, chrome)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
							return c.LayoutPresetChip(gtx, &mcp.PresetFetchButton, "Fetch", enabled, chrome)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
							return c.LayoutPresetChip(gtx, &mcp.PresetCustomButton, "Custom", enabled, chrome)
						})
					}),
				)
			})
		}),
	)
}

func (c *Component) LayoutPresetChip(gtx layout.Context, button *widget.Clickable, label string, enabled bool, chrome Chrome) layout.Dimensions {
	background, foreground, border := chrome.Colors.SurfaceContainerLow, chrome.Colors.OnSurface, chrome.Colors.OutlineVariant
	if button.Hovered() {
		background, border = chrome.Colors.SurfaceContainerHigh, chrome.Colors.Primary
	}
	if !enabled {
		gtx = gtx.Disabled()
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(4), background, border, 1, func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, label, unit.Sp(11), font.SemiBold, foreground, 1)
			})
		})
	})
}
