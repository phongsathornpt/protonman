package runtime

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
	domainmodel "github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestTurnDeltaDrainsBufferedTerminalInOrder(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.busy = true
	m.historyState.StartThinking()
	events := make(chan tea.Msg, 2)
	events <- turnmsg.Delta{Event: app.Event{Kind: app.EventTextDelta, Text: " world"}}
	events <- turnmsg.Done{Result: app.Result{Message: domainmodel.Message{Role: domainmodel.RoleAssistant, Content: "hello world"}}}
	m.turnEvents = events

	cmd := m.updateTurnDelta(turnmsg.Delta{Event: app.Event{Kind: app.EventTextDelta, Text: "hello"}})
	if cmd != nil {
		t.Fatalf("terminal drain returned follow-up command: %v", cmd)
	}
	if m.busy {
		t.Fatal("terminal drain left turn busy")
	}
	plain := plainTranscript(m)
	if !strings.Contains(plain, "hello world") {
		t.Fatalf("drained transcript missing ordered text: %q", plain)
	}
	if len(m.messages) != 1 || m.messages[0].Content != "hello world" {
		t.Fatalf("terminal result messages = %#v", m.messages)
	}
}
