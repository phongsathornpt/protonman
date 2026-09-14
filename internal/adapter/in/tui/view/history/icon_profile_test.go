package history

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuiicon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/icon"
)

func TestSpecializedToolCellsUseProvidedIconProfile(t *testing.T) {
	tests := []struct {
		name string
		cell HistoryCell
		want string
	}{
		{
			name: "exec-running",
			cell: &ExecCell{Name: "bash", Command: "go test ./...", Running: true, Icons: tuiicon.Nerd},
			want: strings.TrimSpace(tuiicon.Nerd.Exec),
		},
		{
			name: "patch-running",
			cell: &PatchCell{Name: "edit", Summary: "update", Running: true, Icons: tuiicon.Nerd},
			want: strings.TrimSpace(tuiicon.Nerd.Edit),
		},
		{
			name: "agent-running",
			cell: &AgentToolCell{Name: "subagent", Running: true, Icons: tuiicon.Nerd},
			want: strings.TrimSpace(tuiicon.Nerd.Agent),
		},
		{
			name: "exec-success",
			cell: &ExecCell{Name: "bash", Command: "true", Icons: tuiicon.Nerd},
			want: strings.TrimSpace(tuiicon.Nerd.ToolSuccess),
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
