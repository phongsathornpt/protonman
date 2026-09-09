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

func TestMinimalWelcomeHidesAfterConversationStarts(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.runner = fakeConversation{}
	m.bottom.prompt().SetValue("hello")
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

func TestMinimalBusyChromeStaysWithinThreeRows(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.showWelcome = false
	m.busy = true
	m.activity = "running tests"
	frame := m.buildFrameChrome()
	if got := lipgloss.Height(frame.status); got != 1 {
		t.Fatalf("busy activity rows=%d, want 1: %q", got, frame.status)
	}
	if frame.top != "" || frame.footer != "" {
		t.Fatalf("busy frame leaked persistent pane/footer: top=%q footer=%q", frame.top, frame.footer)
	}
	if frame.height > 3 {
		t.Fatalf("busy chrome height=%d, want <=3", frame.height)
	}
}

func TestMinimalPromptRestoresEssentialContext(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.bottom.setHasRunner(true)
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
	if !strings.Contains(plain, "ask") {
		t.Fatalf("narrow prompt metadata dropped mode: %q", plain)
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
		m.bottom.push(v)
		rendered := v.Render(m)
		if got := lipgloss.Height(rendered); got > size[1] {
			t.Fatalf("provider model picker height=%d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
		if got := lipgloss.Width(rendered); got > size[0] {
			t.Fatalf("provider model picker width=%d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}
	}
}
