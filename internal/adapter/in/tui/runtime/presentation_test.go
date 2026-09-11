package runtime

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	crashview "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/crash"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/execview"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	applicationturn "github.com/phongsathornpt/protonman/internal/engine/turn"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrandLockupResponsive(t *testing.T) {
	wide := brandLockup(80)
	lines := strings.Split(wide, "\n")
	if len(lines) != 2 {
		t.Fatalf("wide brand lines = %d, want 2: %q", len(lines), wide)
	}
	if !strings.Contains(ansi.Strip(lines[0]), glyphBrand) || !strings.Contains(ansi.Strip(wide), "█▀█") {
		t.Fatalf("wide brand missing mark/ascii wordmark: %q", wide)
	}
	if got := brandLockupWidth(80); got > 80 {
		t.Fatalf("wide brand width = %d, terminal width 80", got)
	}
	narrow := ansi.Strip(brandLockup(20))
	if strings.Contains(narrow, "█") || !strings.Contains(narrow, "protonMAN") {
		t.Fatalf("narrow brand = %q, want compact protonMAN fallback", narrow)
	}
	if got := brandLockupWidth(20); got > 20 {
		t.Fatalf("narrow brand width = %d, terminal width 20", got)
	}
}

func TestBrandMarkIsSingleCell(t *testing.T) {
	if got := ansi.StringWidth(glyphBrand); got != 1 {
		t.Fatalf("brand mark width = %d, want 1", got)
	}
}

func TestLayoutModeBreakpoints(t *testing.T) {
	if got := layoutModeForHeight(24); got != layoutNormal {
		t.Fatalf("24 rows mode = %v", got)
	}
	if got := layoutModeForHeight(18); got != layoutCompact {
		t.Fatalf("18 rows mode = %v", got)
	}
	if got := layoutModeForHeight(12); got != layoutTiny {
		t.Fatalf("12 rows mode = %v", got)
	}
}

func TestPickersFitResponsiveTerminalHeights(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 18}, {40, 14}, {24, 12}} {
		m := newTestSkillsModel(t, 10)
		m.resize(size[0], size[1])
		modelView := newModelSetupPaneView(m).Render(newPaneRenderContext(m))
		if got := lipgloss.Height(modelView); got > size[1] {
			t.Fatalf("model setup height %d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
		if got := lipgloss.Width(modelView); got > size[0] {
			t.Fatalf("model setup width %d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}
		skillsView := (&skillsPaneView{}).Render(newPaneRenderContext(m))
		if got := lipgloss.Height(skillsView); got > size[1] {
			t.Fatalf("skills picker height %d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
		if got := lipgloss.Width(skillsView); got > size[0] {
			t.Fatalf("skills picker width %d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}
	}
}

func TestCompactLayoutReducesChrome(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, []tododomain.Item{{ID: "one", Text: "one", Status: tododomain.StatusPending}})
	m.activeModel = "provider/a-very-long-model-name"
	m.resize(60, 18)
	if strings.Contains(m.promptView(), "╭") || strings.Contains(m.promptView(), "╰") {
		t.Fatalf("compact prompt renders box chrome: %q", m.promptView())
	}
	if got := m.infoView(); strings.Contains(got, "ctrl+p") || strings.Contains(got, "/help") {
		t.Fatalf("compact info leaked shortcut chrome: %q", got)
	}
	m.resize(24, 12)
	if got := lipgloss.Height(m.View().Content); got > 12 {
		t.Fatalf("tiny live view height = %d, want <= 12", got)
	}
}

func TestRunningToolUsesTranscriptAsProgressSurface(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.busy = true
	m.activity = "running read"
	m.turnProgress = turnProgress{Round: 2, ToolCalls: 3}
	m.historyState.StartTool("read")
	if got := m.statusView(); got == "" || !strings.Contains(got, "running read") || !strings.Contains(got, "3 tools") || strings.Contains(got, "round 2") {
		t.Fatalf("running tool status is missing compact progress: %q", got)
	}
	m.historyState.CommitActive()
	m.activity = ""
	if got := m.statusView(); got == "" || strings.Contains(got, "Thinking") {
		t.Fatalf("busy state should use only the global status row: %q", got)
	}
	if transcript := plainTranscript(m); strings.Contains(transcript, "Thinking") {
		t.Fatalf("ephemeral thinking state leaked into transcript: %q", transcript)
	}
}

func TestWelcomeCardContainsBrandOnly(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = "/a/very/long/workspace/path/that/does/not/fit/in/a/narrow/terminal"
	m.activeModel = "provider/a-very-long-model-name-that-does-not-fit"
	m.activeProvider = "provider-name"
	m.resize(32, 14)
	card := m.welcomeCard()
	if !strings.Contains(card, glyphBrand) || !strings.Contains(card, "protonMAN") {
		t.Fatalf("welcome card missing Protonman brand: %q", card)
	}
	for _, unwanted := range []string{m.workDir, m.activeModel, m.activeProvider, "Ask anything", "No model selected"} {
		if unwanted != "" && strings.Contains(card, unwanted) {
			t.Fatalf("welcome card leaked runtime metadata %q: %q", unwanted, card)
		}
	}
	for _, line := range strings.Split(card, "\n") {
		if got := lipgloss.Width(line); got > 32 {
			t.Fatalf("welcome line width = %d, want <= 32: %q", got, line)
		}
	}
}

func TestWelcomeCardNormalModeStaysMinimal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.workDir = "/tmp/test-workspace"
	m.activeModel = "provider/some-model"
	m.resize(80, 24)
	card := m.welcomeCard()
	if !strings.Contains(card, glyphBrand) || !strings.Contains(card, "█▀█") || !strings.Contains(card, "/tmp/test-workspace") {
		t.Fatalf("minimal welcome missing identity or workspace: %q", card)
	}
	for _, unwanted := range []string{"Quick Actions", "/help", "/model", "Tip:", "some-model"} {
		if strings.Contains(card, unwanted) {
			t.Fatalf("minimal welcome leaked %q: %q", unwanted, card)
		}
	}
}

func TestFormatWorkspaceDisplay(t *testing.T) {
	if got := formatWorkspaceDisplay(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		subpath := filepath.Join(home, "projects", "proton")
		if got := formatWorkspaceDisplay(subpath); got != "~/projects/proton" {
			t.Fatalf("expected ~/projects/proton, got %q", got)
		}
	}
}

func TestDetectGitBranch(t *testing.T) {
	tmp := t.TempDir()
	if got := detectGitBranch(tmp); got != "" {
		t.Fatalf("expected empty branch for non-git dir, got %q", got)
	}
	gitDir := filepath.Join(tmp, ".git")
	_ = os.Mkdir(gitDir, 0o755)
	_ = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature-10-out-of-10\n"), 0o644)
	if got := detectGitBranch(tmp); got != "feature-10-out-of-10" {
		t.Fatalf("expected feature-10-out-of-10, got %q", got)
	}
}

func TestTodoToggleOpensFocusedPaneInCompactLayout(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, []tododomain.Item{{ID: "one", Text: "one", Status: tododomain.StatusPending}})
	m.resize(24, 12)
	updated, _ := m.Update(testCtrl('o'))
	m = updated.(*bubbleModel)
	view := m.panes.bottom.find(todoInspectViewID)
	if view == nil {
		t.Fatal("compact todo toggle did not open focused pane")
	}
	got := view.Render(newPaneRenderContext(m))
	if !strings.Contains(got, "one") || !strings.Contains(got, "0/1 done") {
		t.Fatalf("focused todo pane=%q", got)
	}
	if lipgloss.Height(got) > 12 || lipgloss.Width(got) > 24 {
		t.Fatalf("focused todo pane exceeds terminal: %dx%d", lipgloss.Width(got), lipgloss.Height(got))
	}
}

func TestFocusedTodoPaneBoundsAndScrollsLargePlans(t *testing.T) {
	items := make([]tododomain.Item, 100)
	for i := range items {
		items[i] = tododomain.Item{ID: fmt.Sprintf("task-%03d", i), Text: fmt.Sprintf("Task %03d with enough text to exercise truncation", i), Status: tododomain.StatusPending}
	}
	m := newTestBubbleModel(t, permission.ModeAsk, items)
	m.resize(32, 14)
	view := &todoPaneView{}
	first := view.Render(newPaneRenderContext(m))
	if lipgloss.Height(first) > 14 || lipgloss.Width(first) > 32 {
		t.Fatalf("pane exceeds terminal: %dx%d", lipgloss.Width(first), lipgloss.Height(first))
	}
	for range 5 {
		_ = view.HandlePaneKey(newPaneRenderContext(m), testKey(tea.KeyDown))
	}
	after := view.Render(newPaneRenderContext(m))
	if first == after || !strings.Contains(after, "task-005") {
		t.Fatalf("pane did not scroll: %q", after)
	}
}

func TestCoreGlyphsHaveStableSingleCellWidth(t *testing.T) {
	glyphs := map[string]string{"prompt": glyphPrompt, "mark": glyphMark, "success": glyphToolSuccess, "error": glyphToolError, "denied": glyphToolDenied, "web": glyphWeb, "read": glyphRead, "dir": glyphDir, "search": glyphSearch, "exec": glyphExec, "edit": glyphEdit, "skill": glyphSkill, "agent": glyphAgent, "generic": glyphGeneric, "todo_pending": glyphTodoPending, "todo_active": glyphTodoActive}
	for name, glyph := range glyphs {
		if got := ansi.StringWidth(glyph); got != 2 {
			t.Fatalf("%s glyph %q width = %d, want 2 including trailing space", name, glyph, got)
		}
	}
}

func TestExecPresentationGitDiff(t *testing.T) {
	p := execview.Present("git diff", "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n", "")
	if p.Family != execview.FamilyGit || p.Title != "Git diff" {
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
	p := execview.Present("go test ./...", "ok  example/a 0.10s\n?   example/b [no test files]\n", "")
	if p.Title != "Go test ./..." || p.Summary != "1 package passed · 1 no tests" || !p.SuppressRaw {
		t.Fatalf("go test presentation = %#v", p)
	}
}

func TestExecPresentationBunAndNodeTests(t *testing.T) {
	bun := execview.Present("bun test", "  12 pass\n  0 fail\n", "")
	if bun.Title != "Bun test" || bun.Summary != "12 passed" || !bun.SuppressRaw {
		t.Fatalf("bun test presentation = %#v", bun)
	}
	node := execview.Present("node --test", "# tests 9\n# pass 9\n# fail 0\n", "")
	if node.Title != "Node test" || node.Summary != "9 passed" || !node.SuppressRaw {
		t.Fatalf("node test presentation = %#v", node)
	}
}

func TestExecPresentationPromotesFrameworkRunnerOutput(t *testing.T) {
	vite := execview.Present("bun run dev", "VITE v7.0.0 ready in 220 ms\n  Local: http://localhost:5173/\n", "")
	if vite.Family != execview.FamilyVite || vite.Title != "Vite dev" || vite.Summary != "ready" {
		t.Fatalf("vite presentation = %#v", vite)
	}
	next := execview.Present("npm run dev", "▲ Next.js 16.0.0\n- Local: http://localhost:3000\n", "")
	if next.Family != execview.FamilyNext || next.Title != "Next dev" || next.Summary != "ready" {
		t.Fatalf("next presentation = %#v", next)
	}
}

func TestExecPresentationNextBuildRoutes(t *testing.T) {
	output := "▲ Next.js 16.0.0\n○ /\nƒ /dashboard\nƒ /api/users\n"
	p := execview.Present("next build", output, "")
	if p.Title != "Next build" || p.Summary != "3 routes" || len(p.Details) != 3 || !p.SuppressRaw {
		t.Fatalf("next build presentation = %#v", p)
	}
}

func TestExecPresentationGenericFallback(t *testing.T) {
	p := execview.Present("curl https://example.com", "ok", "")
	if p.Family != execview.FamilyGeneric || p.Title != "$ curl https://example.com" || p.SuppressRaw {
		t.Fatalf("generic presentation = %#v", p)
	}
}

func TestExecPresentationGitStatusAndStat(t *testing.T) {
	status := execview.Present("git status --short", " M a.go\n?? b.go\n", "")
	if status.Summary != "1 changed file · 1 untracked" || len(status.Details) != 2 {
		t.Fatalf("git status presentation = %#v", status)
	}
	clean := execview.Present("git status", "On branch develop\nnothing to commit, working tree clean\n", "")
	if clean.Summary != "clean" || len(clean.Details) != 0 {
		t.Fatalf("clean git status presentation = %#v", clean)
	}
	stat := execview.Present("git diff --stat", " a.go | 3 ++-\n b.go | 2 +\n 2 files changed, 3 insertions(+), 2 deletions(-)\n", "")
	if stat.Summary != "2 files · +3 -2" {
		t.Fatalf("git diff stat presentation = %#v", stat)
	}
}

func TestExecPresentationPythonCommands(t *testing.T) {
	pytest := execview.Present("python3 -m pytest tests/", "================ 84 passed, 2 skipped in 1.80s ================\n", "")
	if pytest.Family != execview.FamilyPython || pytest.Title != "Python pytest" || pytest.Summary != "84 passed · 2 skipped" || !pytest.SuppressRaw {
		t.Fatalf("pytest presentation = %#v", pytest)
	}
	eval := execview.Present(`python3 -c "print(1)"`, "1\n", "")
	if eval.Family != execview.FamilyPython || eval.Title != "Python eval" || eval.Action != "eval" {
		t.Fatalf("python eval presentation = %#v", eval)
	}
	unit := execview.Present("python -m unittest", "Ran 36 tests in 0.940s\n\nOK\n", "")
	if unit.Title != "Python unittest" || unit.Summary != "36 passed" || !unit.SuppressRaw {
		t.Fatalf("unittest presentation = %#v", unit)
	}
}

func TestExecPresentationPythonAndNodeCompactTitles(t *testing.T) {
	cases := []struct {
		command string
		title   string
		action  string
	}{{"python3.12 app.py", "Python app.py", "app.py"}, {`python -c "print(1)"`, "Python eval", "eval"}, {"node --test test/router.test.js", "Node test", "test"}, {"node --check src/index.js", "Node check src/index.js", "check"}, {`node -e "console.log(1)"`, "Node eval", "eval"}, {`node -p "process.version"`, "Node print", "print"}}
	for _, tc := range cases {
		p := execview.Present(tc.command, "", "")
		if p.Title != tc.title || p.Action != tc.action {
			t.Fatalf("execview.Present(%q) = %#v", tc.command, p)
		}
	}
}

func TestExecCellGenericSemanticLayout(t *testing.T) {
	exit0, exit1 := 0, 1
	cases := []struct {
		name string
		cell ExecCell
		want []string
	}{{name: "python pytest", cell: ExecCell{Command: "python3 -m pytest", Stdout: "84 passed, 2 skipped in 1.80s\n", ExitCode: &exit0, Duration: 1800 * time.Millisecond}, want: []string{"✓ Python pytest", "84 passed · 2 skipped", "1.8s"}}, {name: "node test failure", cell: ExecCell{Command: "node --test", Stdout: "# pass 18\n# fail 1\nFAIL test/router.test.js\n", ExitCode: &exit1, Duration: 350 * time.Millisecond}, want: []string{"× Node test", "18 passed · 1 failed", "350ms", "FAIL test/router.test.js"}}, {name: "python eval", cell: ExecCell{Command: `python3 -c "print(1)"`, ExitCode: &exit0, Duration: 38 * time.Millisecond}, want: []string{"✓ Python eval", "38ms"}}, {name: "node check", cell: ExecCell{Command: "node --check src/index.js", ExitCode: &exit0, Duration: 42 * time.Millisecond}, want: []string{"✓ Node check src/index.js", "valid syntax", "42ms"}}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered := ansi.Strip(strings.Join(tc.cell.RenderWidth(60), "\n"))
			for _, want := range tc.want {
				if !strings.Contains(rendered, want) {
					t.Fatalf("render missing %q:\n%s", want, rendered)
				}
			}
		})
	}
}

func TestFormatExecDuration(t *testing.T) {
	cases := map[time.Duration]string{220 * time.Millisecond: "220ms", 1800 * time.Millisecond: "1.8s", 74 * time.Second: "1m 14s"}
	for input, want := range cases {
		if got := execview.FormatDuration(input); got != want {
			t.Fatalf("execview.FormatDuration(%s) = %q, want %q", input, got, want)
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
	}{{name: "git status", cell: ExecCell{Command: "git status --short", Stdout: " M a.go\n?? b.go\n", ExitCode: &exit0}, want: []string{"Git status --short", "1 changed file", "1 untracked"}, unwanted: []string{"$ git", "exit 0"}}, {name: "go test", cell: ExecCell{Command: "go test ./...", Stdout: "ok  example/a 0.1s\n?   example/b [no test files]\n", ExitCode: &exit0, Duration: 1800 * time.Millisecond}, want: []string{"Go test ./...", "1 package passed", "1 no tests", "1.8s"}, unwanted: []string{"$ go", "exit 0", "example/a"}}, {name: "bun test", cell: ExecCell{Command: "bun test", Stdout: "12 pass\n0 fail\n", ExitCode: &exit0}, want: []string{"Bun test", "12 passed"}, unwanted: []string{"$ bun", "exit 0"}}, {name: "node test", cell: ExecCell{Command: "node --test", Stdout: "# pass 9\n# fail 0\n", ExitCode: &exit0}, want: []string{"Node test", "9 passed"}, unwanted: []string{"$ node", "exit 0"}}, {name: "vite build", cell: ExecCell{Command: "vite build", Stdout: "✓ 42 modules transformed.\ndist/assets/app.js  80 kB\n", ExitCode: &exit0}, want: []string{"Vite build", "42 modules", "dist/assets/app.js"}, unwanted: []string{"$ vite", "exit 0"}}, {name: "next build", cell: ExecCell{Command: "next build", Stdout: "▲ Next.js 16.0.0\n○ /\nƒ /dashboard\n", ExitCode: &exit0}, want: []string{"Next build", "2 routes", "/dashboard"}, unwanted: []string{"$ next", "exit 0"}}}
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
	registry := newNamedTestRegistry(tool.Definition{Name: "bash", Description: "run shell command", Kind: tool.KindBash, PermissionDetailKey: "command"})
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	m := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, newPermissionBridge(), "")
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
	registry := newNamedTestRegistry(tool.Definition{Name: "bash", Description: "run shell command", Kind: tool.KindBash, PermissionDetailKey: "command"})
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	m := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, newPermissionBridge(), "")
	call, err := tool.NewCall("exec-fail", "bash", []byte(`{"command":"go test ./..."}`))
	if err != nil {
		t.Fatal(err)
	}
	m.appendToolCall(call)
	exit1 := 1
	m.applyToolResult("bash", tool.Result{CallID: "exec-fail", ToolName: "bash", Stdout: "FAIL example/a\n", ExitCode: &exit1, Failure: &tool.Failure{Code: tool.ErrorCodeCommandFailed, Message: "command exited with status 1"}}, nil)
	cells := m.historyState.Cells()
	if len(cells) == 0 {
		t.Fatal("expected completed exec cell")
	}
	cell, ok := cells[len(cells)-1].(*ExecCell)
	if !ok {
		t.Fatalf("completed cell = %T, want *ExecCell", cells[len(cells)-1])
	}
	rendered := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(rendered, "Go test ./...") || !strings.Contains(rendered, "1 package failed") || !strings.Contains(rendered, "FAIL example/a") {
		t.Fatalf("semantic failure render:\n%s", rendered)
	}
}

func TestExecPresentationRustAndCargo(t *testing.T) {
	cargo := execview.Present("cargo test", "test result: ok. 84 passed; 0 failed; 3 ignored; 0 measured; 0 filtered out\n", "")
	if cargo.Title != "Cargo test" || cargo.Summary != "84 passed · 3 ignored" || !cargo.SuppressRaw {
		t.Fatalf("cargo test = %#v", cargo)
	}
	check := execview.Present("cargo check", "warning: unused import\nerror[E0382]: borrow of moved value\n", "")
	if check.Title != "Cargo check" || check.Summary != "1 error · 1 warning" || len(check.Details) == 0 {
		t.Fatalf("cargo check = %#v", check)
	}
	rustc := execview.Present("rustc src/main.rs", "", "")
	if rustc.Title != "Rustc src/main.rs" {
		t.Fatalf("rustc = %#v", rustc)
	}
}

func TestExecPresentationMake(t *testing.T) {
	p := execview.Present("make -j8 test", "", "")
	if p.Title != "Make test" || p.SuccessSummary != "completed" {
		t.Fatalf("make = %#v", p)
	}
	failed := execview.Present("gmake build", "make: *** [Makefile:42: build] Error 1\n", "")
	if failed.Title != "Make build" || len(failed.Details) == 0 {
		t.Fatalf("gmake = %#v", failed)
	}
}

func TestExecPresentationDocker(t *testing.T) {
	build := execview.Present("docker build .", "#12 exporting to image\n", "")
	if build.Title != "Docker build" || build.Summary != "built image" {
		t.Fatalf("docker build = %#v", build)
	}
	compose := execview.Present("docker compose up -d", "Container api Started\nContainer db Started\n", "")
	if compose.Title != "Docker compose up" || compose.Summary != "2 services running" {
		t.Fatalf("docker compose = %#v", compose)
	}
	presentation := execview.Present("docker-compose down", "", "")
	if presentation.Title != "Docker compose down" {
		t.Fatalf("docker-compose = %#v", presentation)
	}
}

func TestExecPresentationJVMTools(t *testing.T) {
	maven := execview.Present("mvn test", "Tests run: 100, Failures: 1, Errors: 0, Skipped: 2\n", "")
	if maven.Title != "Maven test" || maven.Summary != "97 passed · 1 failed · 2 skipped" {
		t.Fatalf("maven = %#v", maven)
	}
	gradle := execview.Present("./gradlew test", "100 tests completed, 2 failed\n", "")
	if gradle.Title != "Gradle test" || gradle.Summary != "98 passed · 2 failed" {
		t.Fatalf("gradle = %#v", gradle)
	}
	javac := execview.Present("javac src/Main.java", "", "")
	if javac.Title != "Javac src/Main.java" {
		t.Fatalf("javac = %#v", javac)
	}
}

func TestExecPresentationPHP(t *testing.T) {
	lint := execview.Present("php -l src/App.php", "No syntax errors detected in src/App.php\n", "")
	if lint.Title != "PHP lint src/App.php" || lint.Summary != "valid syntax" {
		t.Fatalf("php lint = %#v", lint)
	}
	unit := execview.Present("vendor/bin/phpunit", "Tests: 93, Assertions: 120, Failures: 1, Skipped: 1.\n", "")
	if unit.Title != "PHPUnit" || unit.Summary != "91 passed · 1 failed · 1 skipped" {
		t.Fatalf("phpunit = %#v", unit)
	}
	eval := execview.Present(`php -r "echo 1;"`, "1", "")
	if eval.Title != "PHP eval" {
		t.Fatalf("php eval = %#v", eval)
	}
}

func TestExecPresentationRuby(t *testing.T) {
	check := execview.Present("ruby -c app.rb", "Syntax OK\n", "")
	if check.Title != "Ruby check app.rb" || check.Summary != "syntax OK" {
		t.Fatalf("ruby check = %#v", check)
	}
	rspec := execview.Present("bundle exec rspec", "48 examples, 1 failure, 2 pending\n", "")
	if rspec.Title != "RSpec" || rspec.Summary != "48 examples · 1 failure · 2 pending" {
		t.Fatalf("rspec = %#v", rspec)
	}
	eval := execview.Present(`ruby -e "puts 1"`, "1\n", "")
	if eval.Title != "Ruby eval" {
		t.Fatalf("ruby eval = %#v", eval)
	}
}

func TestExecPresentationDotnet(t *testing.T) {
	test := execview.Present("dotnet test", "Passed! - Failed: 0, Passed: 126, Skipped: 2, Total: 128\n", "")
	if test.Title != "Dotnet test" || test.Summary != "126 passed · 2 skipped" {
		t.Fatalf("dotnet test = %#v", test)
	}
	build := execview.Present("dotnet build", "Build succeeded.\n    3 Warning(s)\n    0 Error(s)\n", "")
	if build.Title != "Dotnet build" || build.Summary != "3 warnings" {
		t.Fatalf("dotnet build = %#v", build)
	}
}

func TestExecPresentationTerraform(t *testing.T) {
	plan := execview.Present("terraform plan", "Plan: 3 to add, 1 to change, 0 to destroy.\n", "")
	if plan.Title != "Terraform plan" || plan.Summary != "+3 ~1 -0" {
		t.Fatalf("terraform plan = %#v", plan)
	}
	apply := execview.Present("tofu apply", "Apply complete! Resources: 4 added, 1 changed, 0 destroyed.\n", "")
	if apply.Title != "OpenTofu apply" || apply.Summary != "4 added · 1 changed · 0 destroyed" {
		t.Fatalf("tofu apply = %#v", apply)
	}
	valid := execview.Present("terraform validate", "Success! The configuration is valid.\n", "")
	if valid.Summary != "valid configuration" {
		t.Fatalf("terraform validate = %#v", valid)
	}
}

func TestExecPresentationKubectl(t *testing.T) {
	apply := execview.Present("kubectl apply -f k8s/", "deployment.apps/api configured\nservice/api created\n", "")
	if apply.Title != "Kubectl apply" || apply.Summary != "1 configured · 1 created" {
		t.Fatalf("kubectl apply = %#v", apply)
	}
	get := execview.Present("kubectl get pods", "NAME READY STATUS\na 1/1 Running\nb 1/1 Running\n", "")
	if get.Title != "Kubectl get pods" || get.Summary != "2 resources" || get.SuppressRaw {
		t.Fatalf("kubectl get = %#v", get)
	}
	rollout := execview.Present("kubectl rollout status deployment/api", "deployment \"api\" successfully rolled out\n", "")
	if rollout.Title != "Kubectl rollout status" || rollout.Summary != "rollout complete" {
		t.Fatalf("kubectl rollout = %#v", rollout)
	}
}

func TestExecCellSemanticEcosystemLayouts(t *testing.T) {
	exit0 := 0
	cases := []struct {
		name string
		cell ExecCell
		want []string
	}{{"cargo", ExecCell{Command: "cargo test", Stdout: "test result: ok. 12 passed; 0 failed; 1 ignored; 0 measured; 0 filtered out\n", ExitCode: &exit0, Duration: 4200 * time.Millisecond}, []string{"✓ Cargo test", "12 passed · 1 ignored", "4.2s"}}, {"make", ExecCell{Command: "make build", ExitCode: &exit0, Duration: 1100 * time.Millisecond}, []string{"✓ Make build", "completed", "1.1s"}}, {"docker", ExecCell{Command: "docker compose up -d", Stdout: "Container api Started\nContainer db Started\n", ExitCode: &exit0, Duration: 2800 * time.Millisecond}, []string{"✓ Docker compose up", "2 services running", "2.8s"}}, {"maven", ExecCell{Command: "mvn test", Stdout: "Tests run: 10, Failures: 0, Errors: 0, Skipped: 1\n", ExitCode: &exit0, Duration: 5400 * time.Millisecond}, []string{"✓ Maven test", "9 passed · 1 skipped", "5.4s"}}, {"phpunit", ExecCell{Command: "vendor/bin/phpunit", Stdout: "OK (92 tests, 120 assertions)\n", ExitCode: &exit0, Duration: 1400 * time.Millisecond}, []string{"✓ PHPUnit", "92 passed", "1.4s"}}, {"rspec", ExecCell{Command: "bundle exec rspec", Stdout: "48 examples, 0 failures\n", ExitCode: &exit0, Duration: 780 * time.Millisecond}, []string{"✓ RSpec", "48 examples · 0 failures", "780ms"}}, {"dotnet", ExecCell{Command: "dotnet test", Stdout: "Passed! - Failed: 0, Passed: 126, Skipped: 2, Total: 128\n", ExitCode: &exit0, Duration: 3700 * time.Millisecond}, []string{"✓ Dotnet test", "126 passed · 2 skipped", "3.7s"}}, {"terraform", ExecCell{Command: "terraform plan", Stdout: "Plan: 3 to add, 1 to change, 0 to destroy.\n", ExitCode: &exit0, Duration: 920 * time.Millisecond}, []string{"✓ Terraform plan", "+3 ~1 -0", "920ms"}}, {"kubectl", ExecCell{Command: "kubectl get pods", Stdout: "NAME READY STATUS\na 1/1 Running\nb 1/1 Running\n", ExitCode: &exit0, Duration: 84 * time.Millisecond}, []string{"✓ Kubectl get pods", "2 resources", "84ms", "NAME READY STATUS"}}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered := ansi.Strip(strings.Join(tc.cell.RenderWidth(90), "\n"))
			for _, want := range tc.want {
				if !strings.Contains(rendered, want) {
					t.Fatalf("render missing %q:\n%s", want, rendered)
				}
			}
		})
	}
}

func TestExecProfileWrappersAndEnvPrefixes(t *testing.T) {
	cases := map[string]string{"env RUST_BACKTRACE=1 cargo test": "Cargo test", "command make -j4 test": "Make test", "CI=1 ./gradlew test": "Gradle test", "APP_ENV=test bundle exec rspec": "RSpec", "TF_IN_AUTOMATION=1 tofu plan": "OpenTofu plan"}
	for command, want := range cases {
		if got := execview.Present(command, "", "").Title; got != want {
			t.Fatalf("execview.Present(%q).Title = %q, want %q", command, got, want)
		}
	}
}

func TestClassifyOpenCodeErrorCancellation(t *testing.T) {
	err := context.Canceled
	classified := ClassifyOpenCodeError(err, "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindCancelled {
		t.Fatalf("expected ErrorKindCancelled, got %v", classified.Kind)
	}
	if !classified.Retryable {
		t.Fatal("expected cancellation to be retryable")
	}
}

func TestClassifyOpenCodeErrorUnresolvedToolCall(t *testing.T) {
	err := fmt.Errorf("turn failed: %w: model requested another tool", applicationturn.ErrUnresolvedToolCall)
	classified := ClassifyOpenCodeError(err, "opencode", "model")
	if classified.Kind != ErrorKindToolDispatch {
		t.Fatalf("expected ErrorKindToolDispatch, got %v", classified.Kind)
	}
	if classified.Badge != "TOOL_PROTOCOL" {
		t.Fatalf("badge = %q, want TOOL_PROTOCOL", classified.Badge)
	}
	if !strings.Contains(classified.RawDetails, "unresolved model tool call") {
		t.Fatalf("raw details = %q, want sentinel details", classified.RawDetails)
	}
}

func TestClassifyOpenCodeErrorToolDispatchUnavailable(t *testing.T) {
	err := fmt.Errorf("turn failed: %w: model requested 1 tool call while no tools were available", applicationturn.ErrToolDispatchUnavailable)
	classified := ClassifyOpenCodeError(err, "opencode", "model")
	if classified.Kind != ErrorKindToolDispatch {
		t.Fatalf("expected ErrorKindToolDispatch, got %v", classified.Kind)
	}
	if classified.Badge != "TOOL_DISPATCH" {
		t.Fatalf("badge = %q, want TOOL_DISPATCH", classified.Badge)
	}
	if !strings.Contains(classified.Message, "no tools were available") {
		t.Fatalf("message = %q, want unavailable-tool detail", classified.Message)
	}
}

func TestClassifyOpenCodeErrorOpenCodeModelError(t *testing.T) {
	raw := `provider returned status 401: {"type":"error","error":{"type":"ModelError","message":"Model nonexistent is not supported"}}`
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nonexistent")
	if classified.Kind != ErrorKindModelNotFound {
		t.Fatalf("expected ErrorKindModelNotFound, got %v", classified.Kind)
	}
	if !strings.Contains(classified.Message, "nonexistent") {
		t.Fatalf("expected message to mention nonexistent, got %q", classified.Message)
	}
	if len(classified.Suggestions) == 0 {
		t.Fatal("expected suggestions to be populated")
	}
	if !strings.Contains(classified.Suggestions[0], "/model") {
		t.Fatalf("expected catalog refresh suggestion, got %q", classified.Suggestions[0])
	}
}

func TestClassifyOpenCodeErrorContextOverflow(t *testing.T) {
	tests := []string{"provider returned status 413: request entity too large", `provider returned status 400: {"error":{"code":"context_length_exceeded","message":"Input exceeds context window"}}`, "model error: prompt is too long; exceeded max context length of 128000 tokens", "maximum context length is 128000 tokens, but your request resulted in 130000 tokens", "tokens in request more than max tokens allowed"}
	for _, raw := range tests {
		classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
		if classified.Kind != ErrorKindContextOverflow {
			t.Errorf("for %q: expected ErrorKindContextOverflow, got %v", raw, classified.Kind)
		}
		if len(classified.Suggestions) == 0 {
			t.Errorf("for %q: expected suggestions for context overflow", raw)
		}
		foundClear := false
		for _, s := range classified.Suggestions {
			if strings.Contains(s, "/clear") {
				foundClear = true
				break
			}
		}
		if !foundClear {
			t.Errorf("for %q: expected suggestion mentioning /clear", raw)
		}
	}
}

func TestClassifyOpenCodeErrorAuthentication(t *testing.T) {
	raw := "provider returned status 401: unauthorized: invalid api key"
	classified := ClassifyOpenCodeError(errors.New(raw), "openai", "gpt-4o")
	if classified.Kind != ErrorKindAuthentication {
		t.Fatalf("expected ErrorKindAuthentication, got %v", classified.Kind)
	}
	if classified.Code != "401" {
		t.Fatalf("expected code 401, got %q", classified.Code)
	}
}

func TestClassifyOpenCodeErrorHTMLGateway(t *testing.T) {
	raw := "provider returned status 401: <!DOCTYPE html><html><head><title>401 Authorization Required</title></head><body><h1>401 Authorization Required</h1></body></html>"
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindAuthentication {
		t.Fatalf("expected ErrorKindAuthentication, got %v", classified.Kind)
	}
	if !strings.Contains(classified.Message, "upstream gateway or proxy") {
		t.Fatalf("expected clean gateway message, got %q", classified.Message)
	}
}

func TestClassifyOpenCodeErrorForbidden(t *testing.T) {
	raw := "provider returned status 403: access_denied for requested resource"
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "claude-sonnet-4")
	if classified.Kind != ErrorKindForbidden {
		t.Fatalf("expected ErrorKindForbidden, got %v", classified.Kind)
	}
}

func TestClassifyOpenCodeErrorRateLimitAndQuota(t *testing.T) {
	rawRate := "provider returned status 429: rate limit exceeded. please wait 10 seconds"
	cRate := ClassifyOpenCodeError(errors.New(rawRate), "opencode", "nemotron-3.5-lightning-free")
	if cRate.Kind != ErrorKindRateLimit {
		t.Fatalf("expected ErrorKindRateLimit, got %v", cRate.Kind)
	}
	rawQuota := `provider returned status 429: {"error":{"code":"insufficient_quota","message":"You have exceeded your current quota"}}`
	cQuota := ClassifyOpenCodeError(errors.New(rawQuota), "openai", "gpt-4o")
	if cQuota.Kind != ErrorKindQuotaExceeded {
		t.Fatalf("expected ErrorKindQuotaExceeded, got %v", cQuota.Kind)
	}
}

func TestClassifyOpenCodeErrorServerOverloaded(t *testing.T) {
	raw := "provider returned status 503: Upstream request failed"
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindServerOverloaded {
		t.Fatalf("expected ErrorKindServerOverloaded, got %v", classified.Kind)
	}
	if !classified.Retryable {
		t.Fatal("expected server overloaded to be retryable")
	}
}

func TestClassifyOpenCodeErrorTimeout(t *testing.T) {
	raw := "ProviderHeaderTimeoutError: headers timed out after 30000ms"
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindStreamTimeout {
		t.Fatalf("expected ErrorKindStreamTimeout, got %v", classified.Kind)
	}
}

func TestClassifyOpenCodeErrorMCPFailed(t *testing.T) {
	raw := `MCP server "weather" failed to start`
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindMCPFailed {
		t.Fatalf("expected ErrorKindMCPFailed, got %v", classified.Kind)
	}
	if !strings.Contains(classified.Message, "weather") {
		t.Fatalf("expected message to mention weather, got %q", classified.Message)
	}
}

func TestClassifyOpenCodeErrorConfigErrors(t *testing.T) {
	raw := `ConfigDirectoryTypoError: Directory "prompt" in /path is not valid. Rename the directory to "prompts"`
	classified := ClassifyOpenCodeError(errors.New(raw), "opencode", "nemotron-3.5-lightning-free")
	if classified.Kind != ErrorKindConfigTypo {
		t.Fatalf("expected ErrorKindConfigTypo, got %v", classified.Kind)
	}
}

func TestPickerRenderDoesNotMutateNavigationState(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(80, 24)
	t.Run("model selector", func(t *testing.T) {
		v := &modelSetupPaneView{models: []model.RemoteModel{{ID: "one"}, {ID: "two"}}}
		v.resetSelection("two")
		beforeIndex := v.picker.Index()
		beforePage := v.picker.Paginator.Page
		_ = v.Render(newPaneRenderContext(m))
		if v.picker.Index() != beforeIndex || v.picker.Paginator.Page != beforePage {
			t.Fatalf("Render mutated navigation: index %d→%d page %d→%d", beforeIndex, v.picker.Index(), beforePage, v.picker.Paginator.Page)
		}
	})
}

func TestBuildCrashReport(t *testing.T) {
	report := crashview.BuildCrashReport("nil pointer dereference", "goroutine 1 [running]:\nmain.go:123")
	if !strings.Contains(report, "Protonman Crash Report") {
		t.Fatalf("expected report header, got: %s", report)
	}
	if !strings.Contains(report, "nil pointer dereference") {
		t.Fatalf("expected panic message in report, got: %s", report)
	}
	if !strings.Contains(report, "goroutine 1 [running]") {
		t.Fatalf("expected stack trace in report, got: %s", report)
	}
}

func TestCrashModelNavigation(t *testing.T) {
	stack := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10"
	m := crashview.NewCrashModel("test failure", []byte(stack))
	rendered := m.View().Content
	if !strings.Contains(rendered, "Protonman crashed") {
		t.Fatalf("expected headline in view, got: %s", rendered)
	}
	if !strings.Contains(rendered, "test failure") {
		t.Fatalf("expected error message in view, got: %s", rendered)
	}
	if !strings.Contains(rendered, "[c] Copy report") {
		t.Fatalf("expected copy report action in view, got: %s", rendered)
	}
	_, _ = m.Update(testKey(tea.KeyDown))
	if m.ScrollOffset() != 1 {
		t.Fatalf("expected scrollOffset 1, got %d", m.ScrollOffset())
	}
	_, _ = m.Update(testKey(tea.KeyUp))
	if m.ScrollOffset() != 0 {
		t.Fatalf("expected scrollOffset 0, got %d", m.ScrollOffset())
	}
	_, _ = m.Update(testText("c"))
	if !m.Copied() {
		t.Fatal("expected copied flag to be set")
	}
	_, cmd := m.Update(testText("r"))
	if !m.RestartRequested() {
		t.Fatal("expected restart flag to be set")
	}
	if cmd == nil {
		t.Fatal("expected Quit cmd on restart")
	}
}

func TestCommandHistoryIsBounded(t *testing.T) {
	pane := newBottomPane(true, false)
	for i := 0; i < maxCommandHistory+25; i++ {
		pane.recordHistory(fmt.Sprintf("command-%d", i))
	}
	if got := len(pane.composer.history); got != maxCommandHistory {
		t.Fatalf("history len = %d, want %d", got, maxCommandHistory)
	}
	if got := pane.composer.history[0]; got != "command-25" {
		t.Fatalf("oldest retained history = %q, want command-25", got)
	}
}

func TestDrainQueueClearsDequeuedBackingSlot(t *testing.T) {
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "/tmp/proton")
	m.queue = make([]string, 2, 4)
	m.queue[0] = "/help"
	m.queue[1] = "keep"
	backing := m.queue[:cap(m.queue)]

	_ = m.drainQueue()
	if backing[0] != "" {
		t.Fatalf("dequeued queue slot retained %q", backing[0])
	}
	if len(m.queue) != 1 || m.queue[0] != "keep" {
		t.Fatalf("queue after drain = %#v", m.queue)
	}
}

func TestDrainQueueReleasesBackingWhenEmpty(t *testing.T) {
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "/tmp/proton")
	m.queue = []string{"/help"}
	_ = m.drainQueue()
	if m.queue != nil {
		t.Fatalf("empty queue retained backing slice: %#v", m.queue)
	}
}

func TestQueueFullPreservesDraft(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.busy = true
	for i := 0; i < maxQueuedPrompts; i++ {
		m.queue = append(m.queue, fmt.Sprintf("queued-%d", i))
	}
	m.panes.bottom.prompt().SetValue("keep this draft")
	if cmd := m.submit(); cmd != nil {
		t.Fatalf("submit() command = %v, want nil", cmd)
	}
	if got := m.panes.bottom.prompt().Value(); got != "keep this draft" {
		t.Fatalf("draft = %q, want preserved input", got)
	}
	if got := len(m.queue); got != maxQueuedPrompts {
		t.Fatalf("queue len = %d, want %d", got, maxQueuedPrompts)
	}
}

func TestQueueEchoTruncatesLongPrompt(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.busy = true
	long := strings.Repeat("x", maxQueuePreviewRunes+200)
	m.panes.bottom.prompt().SetValue(long)
	_ = m.submit()
	plain := plainTranscript(m)
	if strings.Contains(plain, long) {
		t.Fatal("queued transcript echoed full long prompt")
	}
	if !strings.Contains(plain, "…") {
		t.Fatalf("queued transcript missing truncation marker: %q", plain)
	}
}

func assertBubbleViewFits(t *testing.T, m *bubbleModel, width, height int) {
	t.Helper()
	m.resize(width, height)
	view := m.View().Content
	if got := lipgloss.Height(view); got > height {
		t.Fatalf("view height %d exceeds %d at %dx%d:\n%s", got, height, width, height, view)
	}
	for _, line := range strings.Split(view, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("line width %d exceeds %d at %dx%d: %q", got, width, width, height, line)
		}
	}
}

func TestResponsiveUXSurfacesFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 18}, {40, 14}, {24, 12}} {
		m := newTestSkillsModel(t, 12)
		m.workDir = "/workspace/a/very/long/path/for/responsive/testing"
		m.activeModel = "provider/a-very-long-model-identifier-for-layout-testing"
		m.activeProvider = "provider-with-a-long-name"
		assertBubbleViewFits(t, m, size[0], size[1])
		m.panes.bottom.push(newModelSetupPaneView(m))
		assertBubbleViewFits(t, m, size[0], size[1])
		m.panes.bottom.remove(modelSetupViewID)
		m.panes.bottom.push(&skillsPaneView{})
		assertBubbleViewFits(t, m, size[0], size[1])
		m.panes.bottom.remove(skillsViewID)
	}
}

func TestPermissionReviewFlowFitsNarrowTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.busy = true
	m.openPermission(permissionRequest{request: permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Detail: "git status --short --branch", Arguments: json.RawMessage(`{"command":"git status --short --branch"}`)}, response: make(chan permissionResponse, 1)})
	assertBubbleViewFits(t, m, 24, 12)
	updated, _ := m.Update(testKey(tea.KeyEsc))
	m = updated.(*bubbleModel)
	assertBubbleViewFits(t, m, 24, 12)
	if !m.permissionView().parked {
		t.Fatal("esc did not enter transcript review mode")
	}
	updated, _ = m.Update(testKey(tea.KeyTab))
	m = updated.(*bubbleModel)
	assertBubbleViewFits(t, m, 24, 12)
	if m.permissionView().parked {
		t.Fatal("tab did not return to permission review")
	}
}

func TestLongActivityStatusFitsTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.busy = true
	m.activity = "connecting to a provider with an absurdly long status description that should never wrap chrome"
	for _, width := range []int{80, 40, 24} {
		m.resize(width, 14)
		if got := ansi.StringWidth(m.statusView()); got > width {
			t.Fatalf("status width %d exceeds terminal width %d: %q", got, width, m.statusView())
		}
	}
}

func TestCompletedTodoPaneIsHidden(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, []tododomain.Item{{ID: "done", Text: "done", Status: tododomain.StatusCompleted}, {ID: "also-done", Text: "also done", Status: tododomain.StatusCompleted}})
	model.resize(80, 24)
	if strings.Contains(model.View().Content, "TODO") {
		t.Fatalf("completed TODO pane still visible: %s", model.View().Content)
	}
}

func TestWelcomeSitsAtTopWithoutFloatingBox(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	view := testPlain(model.View().Content)
	plain := sanitizeBubbleText(view)
	if idx := strings.Index(plain, glyphBrand); idx < 0 || idx > 8 {
		t.Fatalf("welcome is not at the top of the view: %q", plain[:minInt(80, len(plain))])
	}
	if strings.Count(view, "╭") > 1 {
		t.Fatalf("idle view has extra boxes: %s", view)
	}
}

func TestTodoPaneShowsPendingBeforeCompleted(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, []tododomain.Item{{ID: "already-done", Text: "already done", Status: tododomain.StatusCompleted}, {ID: "still-open", Text: "still open", Status: tododomain.StatusPending}, {ID: "also-done", Text: "also done", Status: tododomain.StatusCompleted}})
	model.resize(80, 24)
	model.toggleTodoPane()
	model.reconcileLayout()
	view := testPlain(model.View().Content)
	if !strings.Contains(view, "still open") {
		t.Fatalf("todo pane hid the pending item: %s", view)
	}
	pendingAt := strings.Index(view, "still open")
	doneAt := strings.Index(view, "already done")
	if doneAt >= 0 && pendingAt > doneAt {
		t.Fatal("completed todo rendered before pending todo")
	}
}

func TestResetPromptCollapsesMultilineComposerDuringBusyTurn(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	prompt := model.panes.bottom.prompt()
	prompt.SetValue("one\ntwo\nthree\nfour")
	model.requestRelayout()
	model.reconcileLayout()
	if prompt.Height() != 4 {
		t.Fatalf("multiline prompt height = %d, want 4", prompt.Height())
	}
	model.resetPrompt()
	model.busy = true
	model.activity = ""
	model.reconcileLayout()
	plain := ansi.Strip(model.promptView())
	if got := strings.Count(plain, "> "); got != 1 {
		t.Fatalf("busy composer prompt count = %d, want 1: %q", got, plain)
	}
	if prompt.Height() != 1 {
		t.Fatalf("reset prompt height = %d, want 1", prompt.Height())
	}
}

func TestPromptWidthFitsTerminalAcrossResponsiveSizes(t *testing.T) {
	for _, width := range []int{24, 40, 80, 120} {
		model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		model.resize(width, 16)
		view := model.promptView()
		if got := lipgloss.Width(view); got > width {
			t.Fatalf("prompt width=%d exceeds terminal width=%d: %q", got, width, ansi.Strip(view))
		}
		want := composerUsableWidth(width)
		if got := model.panes.bottom.prompt().Width(); got != want-len(model.panes.bottom.prompt().Prompt) {
			t.Fatalf("textarea content width=%d, want %d at terminal width %d", got, want-len(model.panes.bottom.prompt().Prompt), width)
		}
	}
}

func TestBlankMultilineSubmitCollapsesComposer(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	prompt := model.panes.bottom.prompt()
	prompt.SetValue("\n\n\n")
	model.requestRelayout()
	model.reconcileLayout()
	if prompt.Height() != 4 {
		t.Fatalf("precondition height=%d, want 4", prompt.Height())
	}
	if cmd := model.submit(); cmd != nil {
		t.Fatalf("blank submit command=%v, want nil", cmd)
	}
	if prompt.Value() != "" || prompt.Height() != 1 {
		t.Fatalf("blank submit left value=%q height=%d", prompt.Value(), prompt.Height())
	}
	if got := strings.Count(ansi.Strip(model.promptView()), "> "); got != 1 {
		t.Fatalf("prompt count=%d, want 1: %q", got, ansi.Strip(model.promptView()))
	}
}

func TestComposerNewlineKeyContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "ctrl-enter", key: tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}},
		{name: "ctrl-j", key: testCtrl('j')},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
			model.resize(80, 24)
			model.panes.bottom.prompt().SetValue("hello")
			updated, _ := model.Update(tc.key)
			model = updated.(*bubbleModel)
			if got := model.panes.bottom.prompt().Value(); got != "hello\n" {
				t.Fatalf("composer value = %q, want %q", got, "hello\\n")
			}
			if got := model.panes.bottom.prompt().Height(); got != 2 {
				t.Fatalf("composer height = %d, want 2", got)
			}
			if len(model.panes.bottom.composer.history) != 0 {
				t.Fatalf("newline shortcut submitted composer history: %#v", model.panes.bottom.composer.history)
			}
		})
	}

	t.Run("ctrl-m-is-not-newline", func(t *testing.T) {
		model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		model.panes.bottom.prompt().SetValue("hello")
		updated, _ := model.Update(testCtrl('m'))
		model = updated.(*bubbleModel)
		if got := model.panes.bottom.prompt().Value(); got != "hello" {
			t.Fatalf("ctrl+m changed composer value to %q", got)
		}
	})

	t.Run("enter-submits", func(t *testing.T) {
		model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		model.panes.bottom.prompt().SetValue("hello")
		updated, _ := model.Update(testKey(tea.KeyEnter))
		model = updated.(*bubbleModel)
		if got := model.panes.bottom.prompt().Value(); got != "" {
			t.Fatalf("enter left composer value %q", got)
		}
		if got := model.panes.bottom.composer.history; len(got) != 1 || got[0] != "hello" {
			t.Fatalf("enter did not submit composer history: %#v", got)
		}
	})
}

func TestComposerKeyActionContract(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
		want composerKeyAction
	}{
		{name: "enter", key: testKey(tea.KeyEnter), want: composerKeyActionSubmit},
		{name: "ctrl-enter", key: tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}, want: composerKeyActionNewline},
		{name: "ctrl-j", key: testCtrl('j'), want: composerKeyActionNewline},
		{name: "ctrl-m", key: testCtrl('m'), want: composerKeyActionNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := model.composerAction(tc.key); got != tc.want {
				t.Fatalf("composer action = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestKeyboardEnhancementsPreferCtrlEnterHelp(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	if model.keyboardCapability != keyboardCapabilityUnknown {
		t.Fatalf("initial keyboard capability = %s, want unknown", model.keyboardCapability)
	}
	if got := model.keys.Newline.Help().Key; got != "ctrl+j" {
		t.Fatalf("fallback newline help = %q, want ctrl+j", got)
	}
	updated, _ := model.Update(tea.KeyboardEnhancementsMsg{Flags: 1})
	model = updated.(*bubbleModel)
	if model.keyboardCapability != keyboardCapabilityDisambiguated {
		t.Fatalf("keyboard capability = %s, want disambiguated", model.keyboardCapability)
	}
	if got := model.keys.Newline.Help().Key; got != "ctrl+enter" {
		t.Fatalf("enhanced newline help = %q, want ctrl+enter", got)
	}
	pane := (&shortcutsPaneView{}).Render(newPaneRenderContext(model))
	if plain := ansi.Strip(pane); !strings.Contains(plain, "ctrl+enter") || strings.Contains(plain, "ctrl+j  New line") {
		t.Fatalf("enhanced shortcuts pane did not prefer ctrl+enter: %q", plain)
	}
}

func TestKeyboardEnhancementsRecordLegacyFallback(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	updated, _ := model.Update(tea.KeyboardEnhancementsMsg{})
	model = updated.(*bubbleModel)
	if model.keyboardCapability != keyboardCapabilityLegacy {
		t.Fatalf("keyboard capability = %s, want legacy", model.keyboardCapability)
	}
	if got := model.keys.Newline.Help().Key; got != composerNewlineFallback {
		t.Fatalf("legacy newline help = %q, want %q", got, composerNewlineFallback)
	}
}

func TestNormalizeBlankComposerPolicy(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	prompt := model.panes.bottom.prompt()
	for _, value := range []string{"\n", "   \n", "\n\n\t"} {
		prompt.SetValue(value)
		if !model.normalizeBlankComposer() {
			t.Fatalf("blank composer %q was not normalized", value)
		}
		if prompt.Value() != "" || prompt.Height() != 1 {
			t.Fatalf("normalized composer value=%q height=%d", prompt.Value(), prompt.Height())
		}
	}
	prompt.SetValue("hello\n")
	if model.normalizeBlankComposer() {
		t.Fatal("non-blank multiline composer was normalized")
	}
	if got := prompt.Value(); got != "hello\n" {
		t.Fatalf("non-blank composer changed to %q", got)
	}
}

func TestBlankComposerNewlinesDoNotCreateBorderGap(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)

	for i := 0; i < 3; i++ {
		updated, _ := model.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
		model = updated.(*bubbleModel)
	}

	if got := model.panes.bottom.prompt().Value(); got != "" {
		t.Fatalf("blank multiline value = %q, want empty", got)
	}
	if got := model.panes.bottom.prompt().Height(); got != 1 {
		t.Fatalf("blank multiline height = %d, want 1", got)
	}
	plain := ansi.Strip(model.promptView())
	if got := strings.Count(plain, "> "); got != 1 {
		t.Fatalf("prompt count = %d, want 1: %q", got, plain)
	}
}

func TestPromptIsSingleRow(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	if model.panes.bottom.prompt().Height() != 1 {
		t.Fatalf("prompt height = %d, want 1", model.panes.bottom.prompt().Height())
	}
	if strings.Count(ansi.Strip(model.promptView()), "> ") != 1 {
		t.Fatalf("prompt chrome repeated:\n%s", model.promptView())
	}
}

func TestLiveViewFitsTerminal(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, []tododomain.Item{{ID: "one", Text: "one", Status: tododomain.StatusPending}})
	model.resize(80, 24)
	height := lipgloss.Height(model.View().Content)
	if height > 24 {
		t.Fatalf("view height = %d, want <= 24:\n%s", height, model.View().Content)
	}
}

func TestBubbleModelAcceptsTypedRunes(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	if !model.panes.bottom.prompt().Focused() {
		t.Fatal("prompt is not focused; textarea will drop every key")
	}
	updated, _ := model.Update(testText("h"))
	model = updated.(*bubbleModel)
	updated, _ = model.Update(testText("i"))
	model = updated.(*bubbleModel)
	if got, want := model.panes.bottom.prompt().Value(), "hi"; got != want {
		t.Fatalf("typed value = %q, want %q", got, want)
	}
}

func TestBubbleModelRendersComponentLayout(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	model := newBubbleModel(context.Background(), service, registry, []tododomain.Item{{ID: "ship", Text: "ship Bubble Tea", Status: tododomain.StatusPending}}, nil, newPermissionBridge(), "/tmp/proton")
	model.resize(80, 24)
	model.appendLine("assistant: ready")
	model.refreshViewport()
	view := testPlain(model.View().Content)
	for _, expected := range []string{glyphBrand, "█▀█", "/tmp/proton", "assistant: ready", "> "} {
		if !strings.Contains(view, expected) {
			t.Fatalf("Bubble Tea view does not contain %q: %s", expected, view)
		}
	}
	if strings.Contains(view, "Tasks 0/1") || strings.Contains(view, "ship Bubble Tea") {
		t.Fatalf("main frame still renders persistent task chrome: %s", view)
	}
}

func TestBubbleModelHistoryUsesTextarea(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	model := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, newPermissionBridge(), "")
	model.panes.bottom.prompt().SetValue(":help")
	if command := model.submit(); command != nil {
		t.Fatal("help submit command != nil")
	}
	model.historyPrevious()
	if got, want := model.panes.bottom.prompt().Value(), ":help"; got != want {
		t.Fatalf("history value = %q, want %q", got, want)
	}
	model.historyNext()
	if got := model.panes.bottom.prompt().Value(); got != "" {
		t.Fatalf("history next value = %q, want empty", got)
	}
}

func TestSanitizeBubbleTextRemovesControlCharacters(t *testing.T) {
	got := sanitizeBubbleText("hello\x1b[31m\nworld")
	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("sanitized text contains escape: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("sanitized text contains newline: %q", got)
	}
}

func TestEmptyStateWithoutRunnerGuidesSlashCommands(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	view := testPlain(model.View().Content)
	for _, expected := range []string{"Message or /command", glyphBrand, "█▀█"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("empty state view does not contain %q: %s", expected, view)
		}
	}
	if got, want := model.panes.bottom.prompt().Placeholder, "Message or /command…"; got != want {
		t.Fatalf("placeholder = %q, want %q", got, want)
	}
}

func TestRefreshViewportPreservesScrollWhenNotFollowing(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	for range 40 {
		model.appendLine("line")
	}
	model.refreshViewport()
	model.viewport.GotoTop()
	model.conversationViewport.setFollowing(false)
	model.appendLine("tail")
	model.refreshViewport()
	if model.viewport.AtBottom() {
		t.Fatal("refreshViewport followed the tail after the user scrolled up")
	}
	if model.conversationViewport.following() {
		t.Fatal("followTail was re-enabled after a mid-scroll append")
	}
}

func TestWelcomeCardReprintsAfterClear(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.appendLine("gone")
	model.panes.bottom.prompt().SetValue("/clear")
	_ = model.submit()
	model.refreshViewport()
	view := testPlain(model.View().Content)
	if strings.Contains(plainTranscript(model), "gone") {
		t.Fatal("clear left transcript body")
	}
	if !strings.Contains(view, glyphBrand) || !strings.Contains(view, "█▀█") {
		t.Fatalf("clear did not reprint welcome: %s", view)
	}
}

func TestSpinnerLifecycle(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, nil)
	t.Run("tick returns single tick command without double-batching", func(t *testing.T) {
		model.busy = true
		_, cmd := model.Update(spinner.TickMsg{})
		if cmd == nil {
			t.Fatal("expected non-nil cmd, got nil")
		}
		msg := cmd()
		if _, isBatch := msg.(tea.BatchMsg); isBatch {
			t.Fatal("cmd returned BatchMsg, indicating exponential double-batching of ticks")
		}
	})
}

func TestFormatElapsed(t *testing.T) {
	cases := []struct {
		duration time.Duration
		want     string
	}{{0, "0s"}, {500 * time.Millisecond, "0s"}, {999 * time.Millisecond, "0s"}, {time.Second, "1s"}, {5 * time.Second, "5s"}, {60 * time.Second, "1m0s"}, {61 * time.Second, "1m1s"}}
	for _, tc := range cases {
		got := formatElapsed(tc.duration)
		if got != tc.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", tc.duration, got, tc.want)
		}
	}
}

type fakeConversation struct{}

func (fakeConversation) Run(context.Context, []model.Message, applicationturn.Sink) (applicationturn.Result, error) {
	return applicationturn.Result{}, nil
}

func TestAppendPaneGroupPreservesOverlappingSlices(t *testing.T) {
	rows := []string{"Title", "Run (bash)", "Target: tmp"}
	got := appendPaneGroup(rows[:1], rows[1:]...)
	want := []string{"Title", "", "Run (bash)", "Target: tmp"}
	if len(got) != len(want) {
		t.Fatalf("group length=%d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("group[%d]=%q, want %q: %#v", i, got[i], want[i], got)
		}
	}
}

func TestPromptPlaceholderReflectsRunnerState(t *testing.T) {
	if got := promptPlaceholder(false, permission.ModeAsk, false); got != "Message or /command…" {
		t.Fatalf("no runner placeholder = %q", got)
	}

	for _, tc := range []struct {
		mode permission.Mode
		plan bool
	}{
		{permission.ModeAsk, false},
		{permission.ModeAlwaysApprove, false},
		{permission.ModeDeny, false},
		{permission.ModeAsk, true},
	} {
		if got := promptPlaceholder(true, tc.mode, tc.plan); got != "" {
			t.Fatalf("runner placeholder = %q, want empty", got)
		}
	}

	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.syncPromptPlaceholder()
	if got := m.panes.bottom.prompt().Placeholder; got != "" {
		t.Fatalf("initial runner placeholder = %q, want empty", got)
	}

	_ = m.setPermissionMode(permission.ModeAlwaysApprove)
	m.setPlanEnabled(true)
	if got := m.panes.bottom.prompt().Placeholder; got != "" {
		t.Fatalf("runner placeholder after mode changes = %q, want empty", got)
	}
}

func TestIdleFooterShowsModelReasoningAndPermissionMode(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "nemotron-3.5-lightning-free"
	m.reasoningEffort = sdk.ReasoningDefault
	m.resize(80, 24)
	footer := ansi.Strip(m.idleContextFooter())
	if !strings.Contains(footer, "nemotron-3.5-lightning-free · auto · ask") {
		t.Fatalf("footer missing model/reasoning/permission context: %q", footer)
	}
}

func TestIdleFooterKeepsShortcutHintInAlwaysApprove(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.activeModel = "muse-spark-1.3-contributor-free"
	m.reasoningEffort = sdk.ReasoningDefault
	m.resize(72, 24)
	footer := ansi.Strip(m.idleContextFooter())
	if !strings.Contains(footer, "? for shortcuts") {
		t.Fatalf("footer dropped shortcut hint in always-approve mode: %q", footer)
	}
	if !strings.Contains(footer, " · auto · auto") {
		t.Fatalf("footer did not use compact permission label: %q", footer)
	}
}

func TestPaneKeyboardHelpStaysSingleLine(t *testing.T) {
	for _, width := range []int{24, 32, 40, 60, 80, 120} {
		help := paneKeyboardHelp(width, "↑/↓", "Navigate", "enter", "Select", "tab", "Complete", "esc", "Go Back")
		if got := lipgloss.Height(help); got != 1 {
			t.Fatalf("help height=%d at width=%d, want 1: %q", got, width, help)
		}
		if got := ansi.StringWidth(help); got > width {
			t.Fatalf("help width=%d exceeds %d: %q", got, width, help)
		}
	}
}

func TestModelRetryStatusCountsDownFromRetryDeadline(t *testing.T) {
	now := time.Now()
	retry := sdk.RetryEvent{Phase: sdk.RetryPhaseWaiting, Reason: "incomplete_stream", Attempt: 1, MaxRetries: 2, RetryAt: now.Add(2500 * time.Millisecond)}
	activity, meta, ok := modelRetryStatus(retry, now)
	if !ok || activity != "retrying in 3s" {
		t.Fatalf("activity=%q ok=%v, want countdown", activity, ok)
	}
	if meta != " · retry 1/2 · stream interrupted" {
		t.Fatalf("meta=%q", meta)
	}
	activity, _, ok = modelRetryStatus(retry, now.Add(2200*time.Millisecond))
	if !ok || activity != "retrying in <1s" {
		t.Fatalf("subsecond activity=%q ok=%v", activity, ok)
	}
}

func TestModelRetryStatusShowsCooldownAfterFirstRetry(t *testing.T) {
	now := time.Now()
	retry := sdk.RetryEvent{Phase: sdk.RetryPhaseCooldown, Reason: "first_event_timeout", Attempt: 2, MaxRetries: 2, RetryAt: now.Add(2 * time.Second)}
	activity, meta, ok := modelRetryStatus(retry, now)
	if !ok || activity != "cooling down 2s" {
		t.Fatalf("activity=%q ok=%v, want cooldown countdown", activity, ok)
	}
	if meta != " · retry 2/2 · provider slow" {
		t.Fatalf("meta=%q", meta)
	}
}

func TestIdleContextFooterShowsLowConcurrencyStateForOpenCode(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(100, 24)
	m.activeProvider = model.DefaultOpenCodeName
	m.activeModel = "nemotron-3.5-lightning-free"
	m.lowConcurrencyMode = model.LowConcurrencyOn
	footer := ansi.Strip(m.idleContextFooter())
	if !strings.Contains(footer, "LOW") {
		t.Fatalf("footer missing low concurrency state: %q", footer)
	}
}

func TestIdleContextFooterKeepsLowIndicatorOnNarrowTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(24, 24)
	m.activeProvider = model.DefaultOpenCodeName
	m.activeModel = "nemotron-3.5-lightning-free"
	m.lowConcurrencyMode = model.LowConcurrencyOn
	footer := ansi.Strip(m.idleContextFooter())
	if !strings.Contains(footer, "LOW") {
		t.Fatalf("narrow footer dropped effective low concurrency state: %q", footer)
	}
}

func TestStatusViewKeepsActiveGoalVisible(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(48, 24)
	m.activeGoal = "finish provider-neutral low concurrency mode safely"
	idle := ansi.Strip(m.statusView())
	if !strings.Contains(idle, "Goal") || !strings.Contains(idle, "finish provider-neutral") {
		t.Fatalf("idle status missing active goal: %q", idle)
	}
	m.busy = true
	busy := ansi.Strip(m.statusView())
	if !strings.Contains(busy, "Goal") || !strings.Contains(busy, "finish provider-neutral") {
		t.Fatalf("busy status missing active goal: %q", busy)
	}
}
