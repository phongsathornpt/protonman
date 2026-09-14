package history

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestToolHistoryUsesProvidedIconProfile(t *testing.T) {
	tests := []struct {
		name string
		cell HistoryCell
		want string
	}{
		{
			name: "generic-read",
			cell: &ToolCell{Name: tool.NameRead, ToolKind: tool.KindRead, Running: true, Icons: tuistyle.NerdIcons},
			want: strings.TrimSpace(tuistyle.NerdIcons.Read),
		},
		{
			name: "patch-running",
			cell: &PatchCell{Name: "edit", Summary: "update", Running: true, Icons: tuistyle.NerdIcons},
			want: strings.TrimSpace(tuistyle.NerdIcons.Edit),
		},
		{
			name: "agent-running",
			cell: &AgentToolCell{Name: "subagent", Running: true, Icons: tuistyle.NerdIcons},
			want: strings.TrimSpace(tuistyle.NerdIcons.Agent),
		},
		{
			name: "patch-success",
			cell: &PatchCell{Name: "edit", Summary: "updated", Icons: tuistyle.NerdIcons},
			want: strings.TrimSpace(tuistyle.NerdIcons.ToolSuccess),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain := ansi.Strip(strings.Join(tt.cell.RenderWidth(80), "\n"))
			if !strings.Contains(plain, tt.want) {
				t.Fatalf("rendered cell %q does not contain Nerd glyph %q", plain, tt.want)
			}
		})
	}
}
