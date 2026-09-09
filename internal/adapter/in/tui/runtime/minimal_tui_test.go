package runtime

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestMinimalIdleChromeUsesNoPersistentFooter(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	if got := m.statusView(); got != "" {
		t.Fatalf("idle status = %q, want empty", got)
	}
	if got := m.footerView(); got != "" {
		t.Fatalf("idle footer = %q, want empty", got)
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
