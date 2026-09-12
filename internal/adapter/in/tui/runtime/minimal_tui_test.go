package runtime

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestMinimalIdleChromeUsesContextFooter(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	if got := m.statusView(); got != "" {
		t.Fatalf("idle status = %q, want empty", got)
	}
	footer := ansi.Strip(m.footerView())
	for _, want := range []string{"? for shortcuts", "auto", "ask"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("idle context footer missing %q: %q", want, footer)
		}
	}
}

func TestPlanModeKeepsIdleContextFooter(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "glm-5.3-flash"
	m.reasoningEffort = sdk.ReasoningDefault
	m.resize(80, 24)
	m.setPlanEnabled(true)

	footer := ansi.Strip(m.footerView())
	for _, want := range []string{"? for shortcuts", "auto", "plan"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("plan context footer missing %q: %q", want, footer)
		}
	}
	if strings.Contains(footer, "enter") || strings.Contains(footer, "new line") {
		t.Fatalf("plan mode replaced context footer with shortcut help: %q", footer)
	}
}

func TestContextualHelpUsesBubblesBindings(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.panes.bottom.prompt().SetValue("/he")
	m.syncSlashView()
	slashHelp := ansi.Strip(m.shortcutHint())
	for _, want := range []string{"tab", "accept", "enter", "run", "esc", "close"} {
		if !strings.Contains(slashHelp, want) {
			t.Fatalf("slash help missing %q: %q", want, slashHelp)
		}
	}
	m.panes.bottom.prompt().Reset()
	m.syncSlashView()
	m.panes.bottom.push(&todoPaneView{})
	todoHelp := ansi.Strip(m.shortcutHint())
	for _, want := range []string{"move", "enter/esc", "close"} {
		if !strings.Contains(todoHelp, want) {
			t.Fatalf("todo help missing %q: %q", want, todoHelp)
		}
	}
}

func TestSessionHeaderPersistsAfterConversationStarts(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.runner = fakeConversation{}
	m.panes.bottom.prompt().SetValue("hello")
	_ = m.submit()
	header := ansi.Strip(m.sessionHeaderView())
	if !strings.Contains(header, "protonMAN") {
		t.Fatalf("session header disappeared after conversation started: %q", header)
	}
}

func TestMinimalSmallTerminalFitsAndRendersUnicode(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(40, 10)
	m.appendUser("สวัสดี 日本語")
	m.appendAssistant("ตอบกลับ ภาษาไทย テスト")
	m.refreshViewport()
	view := m.View().Content
	if got := lipgloss.Height(view); got > 10 {
		t.Fatalf("height=%d exceeds terminal height 10", got)
	}
	plain := ansi.Strip(view)
	for _, want := range []string{"สวัสดี", "日本語", "ภาษาไทย", "テスト"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("unicode text %q missing from view: %q", want, plain)
		}
	}
}

func TestMinimalScrollKeepsSingleComposer(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(60, 16)
	m.runner = fakeConversation{}
	m.syncPromptPlaceholder()
	for i := 0; i < 40; i++ {
		m.appendLine("history")
	}
	m.refreshViewport()
	m.viewport.PageUp()
	plain := ansi.Strip(m.View().Content)
	if got := strings.Count(plain, "> "); got != 1 {
		t.Fatalf("composer count=%d, want 1; view=%q", got, plain)
	}
}

func TestMinimalBusyChromeKeepsContextFooterStable(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.activeModel = "glm-5.3-flash"
	m.busy = true
	m.activity = "running tests"
	frame := m.buildFrameLayout()
	if got := lipgloss.Height(frame.status); got != 1 {
		t.Fatalf("busy activity rows=%d, want 1: %q", got, frame.status)
	}
	if frame.top != "" {
		t.Fatalf("busy frame leaked persistent pane: top=%q", frame.top)
	}
	footer := ansi.Strip(frame.footer)
	for _, want := range []string{"? for shortcuts", "auto", "ask"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("busy context footer missing %q: %q", want, footer)
		}
	}
	for _, noise := range []string{"tab queue", "ctrl+c stop"} {
		if strings.Contains(footer, noise) {
			t.Fatalf("busy footer leaked transient help %q: %q", noise, footer)
		}
	}
	if frame.height > 11 {
		t.Fatalf("busy frame height=%d, want <=11", frame.height)
	}
}

func TestScrolledFooterPrioritizesReturnToLatest(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.conversationViewport.setFollowing(false)
	footer := ansi.Strip(m.footerView())
	for _, want := range []string{"enter", "send", "pgdn", "scroll"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("scrolled footer missing %q: %q", want, footer)
		}
	}
}

func TestMinimalComposerKeepsContextInFooter(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.panes.bottom.setHasRunner(true)
	m.activeModel = "glm-5.3-flash"
	m.reasoningEffort = sdk.ReasoningHigh
	m.resize(80, 24)
	prompt := ansi.Strip(m.promptView())
	if !strings.Contains(prompt, "> ") || strings.Contains(prompt, "Message Protonman") || strings.Contains(prompt, "glm-5.3-flash") {
		t.Fatalf("composer should stay visually empty and focused on input: %q", prompt)
	}
	footer := ansi.Strip(m.footerView())
	for _, want := range []string{"? for shortcuts", "high", "ask"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("context footer missing %q: %q", want, footer)
		}
	}
}

func TestMinimalContextFooterFitsNarrowTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "provider/a-very-long-model-name"
	m.reasoningEffort = sdk.ReasoningMedium
	m.resize(24, 12)
	if got := lipgloss.Width(m.footerView()); got > 22 {
		t.Fatalf("footer width=%d exceeds available width: %q", got, m.footerView())
	}
}

func TestProviderEditorModelPickerFitsResponsiveTerminals(t *testing.T) {
	models := make([]model.RemoteModel, 15)
	for i := range models {
		models[i] = model.RemoteModel{ID: "model-" + string(rune('a'+i))}
	}
	for _, size := range [][2]int{{80, 24}, {60, 18}, {40, 14}, {24, 12}} {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.resize(size[0], size[1])
		v := newProviderPaneView()
		v.models = models
		v.state = providerStateSelectModel
		m.panes.bottom.push(v)
		rendered := v.Render(newPaneRenderContext(m))
		if got := lipgloss.Height(rendered); got > size[1] {
			t.Fatalf("provider model picker height=%d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
		if got := lipgloss.Width(rendered); got > size[0] {
			t.Fatalf("provider model picker width=%d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}
	}
}

func TestMinimalBusyStatusPrefersActiveToolName(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.busy = true
	m.activity = ""
	m.ensureHistoryState().StartToolCell(&ToolCell{CallID: "tool-1", Name: "read", Target: "internal/tui.go", Running: true})
	plain := ansi.Strip(m.statusView())
	for _, want := range []string{"read", "internal/tui.go"} {
		if !strings.Contains(strings.ToLower(plain), strings.ToLower(want)) {
			t.Fatalf("busy status missing %q: %q", want, plain)
		}
	}
}

func TestMinimalIdleStatusDoesNotReuseAssistantGlyph(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.busy = true
	m.activity = ""
	m.spinner.Spinner.Frames = nil
	plain := ansi.Strip(m.statusView())
	if strings.HasPrefix(plain, "● ") {
		t.Fatalf("busy status reused assistant glyph: %q", plain)
	}
}

func TestMinimalLayoutFitsCommonTerminalWidths(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {60, 16}, {80, 24}, {120, 32}} {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.runner = fakeConversation{}
		m.panes.bottom.setHasRunner(true)
		m.activeModel = "glm-5.3-flash"
		m.agentProfile = "engineer"
		m.workDir = "/workspace/protonman"
		m.resize(size[0], size[1])
		view := m.View().Content
		if got := lipgloss.Width(view); got > size[0] {
			t.Fatalf("view width=%d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}
		if got := lipgloss.Height(view); got > size[1] {
			t.Fatalf("view height=%d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
	}
}

func TestMinimalIdleFooterHidesSecondaryShortcuts(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	footer := ansi.Strip(m.footerView())
	for _, noise := range []string{"clear", "quit", "todos", "transcript", "mode:", "skills"} {
		if strings.Contains(footer, noise) {
			t.Fatalf("idle footer leaked secondary shortcut %q: %q", noise, footer)
		}
	}
}

func TestViewIsPureAndIdempotent(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(60, 16)
	m.appendLine("history")
	m.refreshViewport()

	generation := m.layout.generation
	frame := m.layout.frame
	yOffset := m.viewport.YOffset()

	first := m.View().Content
	second := m.View().Content
	if first != second {
		t.Fatal("repeated View calls produced different output")
	}
	if m.layout.generation != generation || m.layout.frame != frame || m.viewport.YOffset() != yOffset {
		t.Fatal("View mutated runtime state")
	}
}

func TestMinimalVeryNarrowUnicodeFrameStaysWithinTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(16, 8)
	m.appendUser("ภาษาไทย 👨‍💻 e\u0301 東京")
	m.appendAssistant("ตอบกลับ テスト café")
	m.refreshViewport()
	view := m.View().Content
	if got := lipgloss.Height(view); got > 8 {
		t.Fatalf("height=%d exceeds terminal height 8", got)
	}
	for _, line := range strings.Split(view, "\n") {
		if got := ansi.StringWidth(line); got > 16 {
			t.Fatalf("line width=%d exceeds terminal width 16: %q", got, line)
		}
	}
}

func TestIdleQuestionMarkOpensShortcutPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	updated, _ := m.Update(testText("?"))
	m = updated.(*bubbleModel)
	if !m.panes.bottom.has(shortcutsViewID) {
		t.Fatal("? did not open shortcuts pane")
	}
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"Shortcuts", "ctrl+p", "Switch model", "ctrl+t", "Transcript", "esc/?", "go back"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("shortcut pane missing %q: %q", want, plain)
		}
	}
	if got := m.panes.bottom.prompt().Value(); got != "" {
		t.Fatalf("? leaked into composer: %q", got)
	}
}

func TestSessionHeaderStaysFixedWhileScrolling(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "qwen3.8-27b"
	m.activeGoal = "refactor TUI branding"
	m.lowConcurrencyMode = model.LowConcurrencyOn
	m.resize(80, 24)
	for i := 0; i < 40; i++ {
		m.appendLine("history")
	}
	m.refreshViewport()
	before := ansi.Strip(m.layout.frame.header)
	m.viewport.PageUp()
	m.conversationViewport.setFollowing(false)
	after := ansi.Strip(m.View().Content)
	if before == "" || !strings.HasPrefix(after, before) {
		t.Fatalf("scrolling moved or removed fixed session header: header=%q view=%q", before, after)
	}
}

func TestClearConversationKeepsSessionHeader(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "qwen3.8-27b"
	m.activeGoal = "refactor TUI branding"
	m.lowConcurrencyMode = model.LowConcurrencyOn
	m.resize(80, 24)
	m.appendLine("history")
	m.clearConversation()
	header := ansi.Strip(m.sessionHeaderView())
	for _, want := range []string{"protonMAN", "qwen3.8-27b", "low", "goal active"} {
		if !strings.Contains(header, want) {
			t.Fatalf("clear removed session header state %q: %q", want, header)
		}
	}
}
