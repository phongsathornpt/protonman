package toolview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestProjectHeaderStateGrammar(t *testing.T) {
	tests := []struct {
		name  string
		input HeaderInput
		state HeaderState
		meta  string
	}{
		{name: "running", input: HeaderInput{Name: tool.NameRead, Kind: tool.KindRead, Target: "main.go", Running: true}, state: HeaderRunning, meta: "…"},
		{name: "success", input: HeaderInput{Name: tool.NameRead, Kind: tool.KindRead, Target: "main.go", Summary: "12 lines (1.2 KB)"}, state: HeaderSuccess, meta: "12 lines (1.2 KB)"},
		{name: "denied", input: HeaderInput{Name: tool.NameRead, Kind: tool.KindRead, Target: "secret.txt", Denied: true}, state: HeaderDenied, meta: "denied"},
		{name: "failure", input: HeaderInput{Name: tool.NameRead, Kind: tool.KindRead, Target: "missing.go", FailureCode: tool.ErrorCodeNotFound}, state: HeaderFailure, meta: "file not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectHeader(tt.input)
			if got.State != tt.state || got.Meta != tt.meta {
				t.Fatalf("header = %#v, want state %v meta %q", got, tt.state, tt.meta)
			}
			if got.Label != "Read" {
				t.Fatalf("label = %q, want Read", got.Label)
			}
		})
	}
}

func TestFailureLabelIsHumanFacing(t *testing.T) {
	tests := []struct {
		name string
		kind tool.Kind
		code tool.ErrorCode
		want string
	}{
		{name: tool.NameRead, kind: tool.KindRead, code: tool.ErrorCodeNotFound, want: "file not found"},
		{name: tool.NameLS, kind: tool.KindRead, code: tool.ErrorCodeNotFound, want: "path not found"},
		{name: "bash", kind: tool.KindBash, code: tool.ErrorCodeDeadlineExceeded, want: "timed out"},
		{name: "edit", kind: tool.KindEdit, code: tool.ErrorCodeOutsideWorkspace, want: "outside workspace"},
	}
	for _, tt := range tests {
		if got := FailureLabel(tt.name, tt.kind, tt.code); got != tt.want {
			t.Fatalf("FailureLabel(%q, %q, %q) = %q, want %q", tt.name, tt.kind, tt.code, got, tt.want)
		}
	}
}

func TestRenderHeaderKeepsSemanticOrderAndWidth(t *testing.T) {
	header := ProjectHeader(HeaderInput{
		Name:        tool.NameRead,
		Kind:        tool.KindRead,
		Target:      "internal/base/runtimepolicy/does-not-exist.go",
		FailureCode: tool.ErrorCodeNotFound,
	})
	plain := ansi.Strip(RenderHeader(header, 42))
	if !strings.HasPrefix(plain, "× Read ") || !strings.Contains(plain, "file not found") {
		t.Fatalf("rendered header = %q", plain)
	}
	if width := ansi.StringWidth(RenderHeader(header, 42)); width > 42 {
		t.Fatalf("header width = %d, want <= 42", width)
	}
}

func TestRenderHeaderUsesStateStyles(t *testing.T) {
	running := RenderHeader(ProjectHeader(HeaderInput{Name: tool.NameRead, Kind: tool.KindRead, Running: true}), 80)
	if !strings.Contains(running, tuistyle.FocusStyle.Render(KindGlyph(tool.KindRead, tool.NameRead))) {
		t.Fatalf("running header does not use focus style: %q", running)
	}

	denied := RenderHeader(ProjectHeader(HeaderInput{Name: tool.NameRead, Kind: tool.KindRead, Denied: true}), 80)
	if !strings.Contains(denied, tuistyle.ErrorStyle.Render(tuistyle.GlyphToolDenied)) {
		t.Fatalf("denied header does not use error style: %q", denied)
	}

	success := RenderHeader(ProjectHeader(HeaderInput{Name: tool.NameRead, Kind: tool.KindRead}), 80)
	if !strings.Contains(success, tuistyle.SuccessStyle.Render(tuistyle.GlyphToolSuccess)) {
		t.Fatalf("success header does not use success style: %q", success)
	}
}

func TestProjectHeaderAllowsSpecializedLabel(t *testing.T) {
	header := ProjectHeader(HeaderInput{Name: tool.NameSkill, Label: "Activated skill", Target: `"pdf-processing"`})
	if header.Label != "Activated skill" || header.Target != `"pdf-processing"` {
		t.Fatalf("header = %#v", header)
	}
}
