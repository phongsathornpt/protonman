package runtime

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestMinimalIdleChromeUsesBubblesHelp(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	if got := m.statusView(); got != "" {
		t.Fatalf("idle status = %q, want empty", got)
	}
	footer := ansi.Strip(m.footerView())
	for _, want := range []string{"enter", "send", "ctrl+j", "newline"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("idle bubbles help missing %q: %q", want, footer)
		}
	}
	if got := m.infoView(); got != "" {
		t.Fatalf("idle info = %q, want empty", got)
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

func TestMinimalWelcomeHidesAfterConversationStarts(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.runner = fakeConversation{}
	m.panes.bottom.prompt().SetValue("hello")
	_ = m.submit()
	if m.showWelcome {
		t.Fatal("welcome remained visible after conversation started")
	}
}

func TestMinimalSmallTerminalFitsAndRendersUnicode(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(40, 10)
	m.showWelcome = false
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
	m.showWelcome = false
	for i := 0; i < 40; i++ {
		m.appendLine("history")
	}
	m.refreshViewport()
	m.viewport.PageUp()
	plain := ansi.Strip(m.View().Content)
	if got := strings.Count(plain, "Message Protonman"); got != 1 {
		t.Fatalf("composer count=%d, want 1; view=%q", got, plain)
	}
}

func TestMinimalBusyChromeKeepsActionableHelpCompact(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.showWelcome = false
	m.busy = true
	m.activity = "running tests"
	frame := m.buildFrameChrome()
	if got := lipgloss.Height(frame.status); got != 1 {
		t.Fatalf("busy activity rows=%d, want 1: %q", got, frame.status)
	}
	if frame.top != "" {
		t.Fatalf("busy frame leaked persistent pane: top=%q", frame.top)
	}
	footer := ansi.Strip(frame.footer)
	for _, want := range []string{"tab", "queue", "ctrl+c", "stop"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("busy help missing %q: %q", want, footer)
		}
	}
	if frame.height > 4 {
		t.Fatalf("busy chrome height=%d, want <=4", frame.height)
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

func TestMinimalPromptRestoresEssentialContext(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.panes.bottom.setHasRunner(true)
	m.activeModel = "glm-5.3-flash"
	m.agentProfile = "engineer"
	m.workDir = "/tmp/protonman"
	m.resize(80, 24)
	plain := ansi.Strip(m.promptView())
	for _, want := range []string{"glm-5.3-flash", "engineer", "/tmp/protonman", "ask", "Message Protonman"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("prompt context %q missing from %q", want, plain)
		}
	}
}

func TestMinimalPromptMetadataFitsNarrowTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "provider/a-very-long-model-name"
	m.agentProfile = "engineer"
	m.workDir = "/workspace/a/very/long/path"
	m.resize(40, 12)
	plain := ansi.Strip(m.promptMetadataView())
	for _, want := range []string{"a-very-long-model-name", "path", "ask"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("narrow prompt metadata dropped %q: %q", want, plain)
		}
	}
	for _, line := range strings.Split(m.promptView(), "\n") {
		if got := lipgloss.Width(line); got > 40 {
			t.Fatalf("prompt line width=%d exceeds 40: %q", got, line)
		}
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
	m.showWelcome = false
	m.busy = true
	m.activity = "analyzing"
	m.ensureHistoryState().StartToolCell(&ToolCell{CallID: "tool-1", Name: "read", Target: "internal/tui.go", Running: true})
	plain := ansi.Strip(m.statusView())
	for _, want := range []string{"read", "internal/tui.go"} {
		if !strings.Contains(strings.ToLower(plain), strings.ToLower(want)) {
			t.Fatalf("busy status missing %q: %q", want, plain)
		}
	}
}

func TestMinimalPromptMetadataUsesDisplayWidth(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "模型-ภาษาไทย"
	m.agentProfile = "工程"
	m.workDir = "/tmp/โครงการ"
	m.resize(32, 12)
	for _, line := range strings.Split(m.promptMetadataView(), "\n") {
		if got := lipgloss.Width(line); got > 32 {
			t.Fatalf("metadata width=%d exceeds 32: %q", got, line)
		}
	}
}

func TestMinimalIdleStatusDoesNotReuseAssistantGlyph(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.busy = true
	m.activity = "analyzing"
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
	for _, noise := range []string{"clear", "quit", "todos", "transcript", "mode", "skills", "model"} {
		if strings.Contains(footer, noise) {
			t.Fatalf("idle footer leaked secondary shortcut %q: %q", noise, footer)
		}
	}
}

func TestViewIsPureAndIdempotent(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(60, 16)
	m.showWelcome = false
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
	m.showWelcome = false
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
