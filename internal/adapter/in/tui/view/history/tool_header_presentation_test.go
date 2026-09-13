package history

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestToolCellFailureHeaderUsesHumanFacingGrammar(t *testing.T) {
	cell := ToolCell{
		Name:        tool.NameRead,
		Target:      "internal/base/runtimepolicy/does-not-exist.go",
		ToolKind:    tool.KindRead,
		FailureCode: tool.ErrorCodeNotFound,
		Body:        "missing",
	}

	lines := cell.RenderWidth(80)
	if len(lines) == 0 {
		t.Fatal("RenderWidth returned no lines")
	}
	primary := ansi.Strip(lines[0])
	for _, want := range []string{"× Read", "does-not-exist.go", "file not found"} {
		if !strings.Contains(primary, want) {
			t.Fatalf("primary header %q missing %q", primary, want)
		}
	}
	if strings.Contains(primary, string(tool.ErrorCodeNotFound)) {
		t.Fatalf("primary header leaked internal failure code: %q", primary)
	}
}

func TestToolCellHeaderStateGrammar(t *testing.T) {
	tests := []struct {
		name string
		cell ToolCell
		want []string
	}{
		{
			name: "running",
			cell: ToolCell{Name: tool.NameRead, Target: "main.go", ToolKind: tool.KindRead, Running: true},
			want: []string{"Read", "main.go", "…"},
		},
		{
			name: "success",
			cell: ToolCell{Name: tool.NameRead, Target: "main.go", ToolKind: tool.KindRead, Summary: "12 lines (1.2 KB)"},
			want: []string{"✓ Read", "main.go", "12 lines (1.2 KB)"},
		},
		{
			name: "denied",
			cell: ToolCell{Name: tool.NameRead, Target: "secret.txt", ToolKind: tool.KindRead, Denied: true},
			want: []string{"! Read", "secret.txt", "denied"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := tt.cell.RenderWidth(80)
			if len(lines) == 0 {
				t.Fatal("RenderWidth returned no lines")
			}
			primary := ansi.Strip(lines[0])
			for _, want := range tt.want {
				if !strings.Contains(primary, want) {
					t.Fatalf("primary header %q missing %q", primary, want)
				}
			}
		})
	}
}
