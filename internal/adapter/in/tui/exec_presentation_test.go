package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/tool"
)

func TestExecPresentationGitDiff(t *testing.T) {
	p := presentExec("git diff", "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n", "")
	if p.Family != execFamilyGit || p.Title != "Git diff" {
		t.Fatalf("presentation = %#v", p)
	}
	if p.Summary != "1 file · +1 -1" || !p.SuppressRaw {
		t.Fatalf("git diff summary = %#v", p)
	}
	if got := strings.Join(p.Details, "\n"); !strings.Contains(got, "diff --git") || !strings.Contains(got, "@@") {
		t.Fatalf("git diff details = %q", got)
	}
}

func TestExecPresentationGoTest(t *testing.T) {
	p := presentExec("go test ./...", "ok  example/a 0.10s\n?   example/b [no test files]\n", "")
	if p.Title != "Go test ./..." || p.Summary != "1 package passed · 1 no tests" || !p.SuppressRaw {
		t.Fatalf("go test presentation = %#v", p)
	}
}

func TestExecPresentationBunAndNodeTests(t *testing.T) {
	bun := presentExec("bun test", "  12 pass\n  0 fail\n", "")
	if bun.Title != "Bun test" || bun.Summary != "12 passed" || !bun.SuppressRaw {
		t.Fatalf("bun test presentation = %#v", bun)
	}
	node := presentExec("node --test", "# tests 9\n# pass 9\n# fail 0\n", "")
	if node.Title != "Node --test" || node.Summary != "9 passed" || !node.SuppressRaw {
		t.Fatalf("node test presentation = %#v", node)
	}
}

func TestExecPresentationPromotesFrameworkRunnerOutput(t *testing.T) {
	vite := presentExec("bun run dev", "VITE v7.0.0 ready in 220 ms\n  Local: http://localhost:5173/\n", "")
	if vite.Family != execFamilyVite || vite.Title != "Vite dev" || vite.Summary != "ready" {
		t.Fatalf("vite presentation = %#v", vite)
	}
	next := presentExec("npm run dev", "▲ Next.js 16.0.0\n- Local: http://localhost:3000\n", "")
	if next.Family != execFamilyNext || next.Title != "Next dev" || next.Summary != "ready" {
		t.Fatalf("next presentation = %#v", next)
	}
}

func TestExecPresentationNextBuildRoutes(t *testing.T) {
	output := "▲ Next.js 16.0.0\n○ /\nƒ /dashboard\nƒ /api/users\n"
	p := presentExec("next build", output, "")
	if p.Title != "Next build" || p.Summary != "3 routes" || len(p.Details) != 3 || !p.SuppressRaw {
		t.Fatalf("next build presentation = %#v", p)
	}
}

func TestExecPresentationGenericFallback(t *testing.T) {
	p := presentExec("curl https://example.com", "ok", "")
	if p.Family != execFamilyGeneric || p.Title != "$ curl https://example.com" || p.SuppressRaw {
		t.Fatalf("generic presentation = %#v", p)
	}
}

func TestExecPresentationGitStatusAndStat(t *testing.T) {
	status := presentExec("git status --short", " M a.go\n?? b.go\n", "")
	if status.Summary != "1 changed file · 1 untracked" || len(status.Details) != 2 {
		t.Fatalf("git status presentation = %#v", status)
	}
	clean := presentExec("git status", "On branch develop\nnothing to commit, working tree clean\n", "")
	if clean.Summary != "clean" || len(clean.Details) != 0 {
		t.Fatalf("clean git status presentation = %#v", clean)
	}
	stat := presentExec("git diff --stat", " a.go | 3 ++-\n b.go | 2 +\n 2 files changed, 3 insertions(+), 2 deletions(-)\n", "")
	if stat.Summary != "2 files · +3 -2" {
		t.Fatalf("git diff stat presentation = %#v", stat)
	}
}

func TestFormatExecDuration(t *testing.T) {
	cases := map[time.Duration]string{
		220 * time.Millisecond:  "220ms",
		1800 * time.Millisecond: "1.8s",
		74 * time.Second:        "1m 14s",
	}
	for input, want := range cases {
		if got := formatExecDuration(input); got != want {
			t.Fatalf("formatExecDuration(%s) = %q, want %q", input, got, want)
		}
	}
}

func TestExecCellSemanticFamilyHeaders(t *testing.T) {
	exit0 := 0
	cases := []struct {
		name     string
		cell     ExecCell
		want     []string
		unwanted []string
	}{
		{
			name:     "git status",
			cell:     ExecCell{Command: "git status --short", Stdout: " M a.go\n?? b.go\n", ExitCode: &exit0},
			want:     []string{"Git status --short", "1 changed file", "1 untracked"},
			unwanted: []string{"$ git", "exit 0"},
		},
		{
			name:     "go test",
			cell:     ExecCell{Command: "go test ./...", Stdout: "ok  example/a 0.1s\n?   example/b [no test files]\n", ExitCode: &exit0, Duration: 1800 * time.Millisecond},
			want:     []string{"Go test ./...", "1 package passed", "1 no tests", "1.8s"},
			unwanted: []string{"$ go", "exit 0", "example/a"},
		},
		{
			name:     "bun test",
			cell:     ExecCell{Command: "bun test", Stdout: "12 pass\n0 fail\n", ExitCode: &exit0},
			want:     []string{"Bun test", "12 passed"},
			unwanted: []string{"$ bun", "exit 0"},
		},
		{
			name:     "node test",
			cell:     ExecCell{Command: "node --test", Stdout: "# pass 9\n# fail 0\n", ExitCode: &exit0},
			want:     []string{"Node --test", "9 passed"},
			unwanted: []string{"$ node", "exit 0"},
		},
		{
			name:     "vite build",
			cell:     ExecCell{Command: "vite build", Stdout: "✓ 42 modules transformed.\ndist/assets/app.js  80 kB\n", ExitCode: &exit0},
			want:     []string{"Vite build", "42 modules", "dist/assets/app.js"},
			unwanted: []string{"$ vite", "exit 0"},
		},
		{
			name:     "next build",
			cell:     ExecCell{Command: "next build", Stdout: "▲ Next.js 16.0.0\n○ /\nƒ /dashboard\n", ExitCode: &exit0},
			want:     []string{"Next build", "2 routes", "/dashboard"},
			unwanted: []string{"$ next", "exit 0"},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rendered := ansi.Strip(strings.Join(tt.cell.RenderWidth(100), "\n"))
			for _, want := range tt.want {
				if !strings.Contains(rendered, want) {
					t.Fatalf("render missing %q:\n%s", want, rendered)
				}
			}
			for _, unwanted := range tt.unwanted {
				if strings.Contains(rendered, unwanted) {
					t.Fatalf("render unexpectedly contains %q:\n%s", unwanted, rendered)
				}
			}
		})
	}
}

func TestBashToolUsesStructuredExecCell(t *testing.T) {
	registry := newNamedTestRegistry(tool.Definition{
		Name:                "bash",
		Description:         "run shell command",
		Kind:                tool.KindBash,
		PermissionDetailKey: "command",
	})
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	m := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		nil,
		newPermissionBridge(),
		"",
	)
	call, err := tool.NewCall("exec-1", "bash", []byte(`{"command":"go test ./..."}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	m.appendToolCall(call)

	cell, ok := m.historyState.Active().(*ExecCell)
	if !ok {
		t.Fatalf("active cell = %T, want *ExecCell", m.historyState.Active())
	}
	if cell.Command != "go test ./..." || !cell.Running {
		t.Fatalf("exec cell = %#v", cell)
	}
}

func TestBashCommandFailureKeepsExecCellPresentation(t *testing.T) {
	registry := newNamedTestRegistry(tool.Definition{
		Name: "bash", Description: "run shell command", Kind: tool.KindBash, PermissionDetailKey: "command",
	})
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	m := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, newPermissionBridge(), "")
	call, err := tool.NewCall("exec-fail", "bash", []byte(`{"command":"go test ./..."}`))
	if err != nil {
		t.Fatal(err)
	}
	m.appendToolCall(call)
	exit1 := 1
	m.applyToolResult("bash", tool.Result{
		CallID: "exec-fail", ToolName: "bash", Stdout: "FAIL example/a\n", ExitCode: &exit1,
		Failure: &tool.Failure{Code: tool.ErrorCodeCommandFailed, Message: "command exited with status 1"},
	}, nil)
	cells := m.historyState.Cells()
	if len(cells) == 0 {
		t.Fatal("expected completed exec cell")
	}
	cell, ok := cells[len(cells)-1].(*ExecCell)
	if !ok {
		t.Fatalf("completed cell = %T, want *ExecCell", cells[len(cells)-1])
	}
	rendered := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(rendered, "Go test ./...") || !strings.Contains(rendered, "exit 1") || !strings.Contains(rendered, "FAIL example/a") {
		t.Fatalf("semantic failure render:\n%s", rendered)
	}
}
