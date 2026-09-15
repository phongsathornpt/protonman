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
			cell: &ToolCell{Name: tool.NameRead, ToolKind: tool.KindRead, Running: true, Icons: tuistyle.UnicodeIcons},
			want: strings.TrimSpace(tuistyle.UnicodeIcons.Read),
		},
		{
			name: "exec-running",
			cell: &ExecCell{Name: "bash", Command: "go test ./...", Running: true, Icons: tuistyle.UnicodeIcons},
			want: strings.TrimSpace(tuistyle.UnicodeIcons.Exec),
		},
		{
			name: "exec-success",
			cell: &ExecCell{Name: "bash", Command: "true", Icons: tuistyle.UnicodeIcons},
			want: strings.TrimSpace(tuistyle.UnicodeIcons.ToolSuccess),
		},
		{
			name: "patch-running",
			cell: &PatchCell{Name: "edit", Summary: "update", Running: true, Icons: tuistyle.UnicodeIcons},
			want: strings.TrimSpace(tuistyle.UnicodeIcons.Edit),
		},
		{
			name: "agent-running",
			cell: &AgentToolCell{Name: "subagent", Running: true, Icons: tuistyle.UnicodeIcons},
			want: strings.TrimSpace(tuistyle.UnicodeIcons.Agent),
		},
		{
			name: "patch-success",
			cell: &PatchCell{Name: "edit", Summary: "updated", Icons: tuistyle.UnicodeIcons},
			want: strings.TrimSpace(tuistyle.UnicodeIcons.ToolSuccess),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain := ansi.Strip(strings.Join(tt.cell.RenderWidth(80), "\n"))
			if !strings.Contains(plain, tt.want) {
				t.Fatalf("rendered cell %q does not contain icon %q", plain, tt.want)
			}
		})
	}
}

func TestExecRawLinesRemainPortable(t *testing.T) {
	cell := ExecCell{Name: "bash", Command: "go test ./...", Icons: tuistyle.UnicodeIcons}
	raw := strings.Join(cell.RawLines(), "\n")
	if !strings.HasPrefix(raw, "$ go test ./...") {
		t.Fatalf("raw exec transcript = %q, want portable shell prefix", raw)
	}
}
