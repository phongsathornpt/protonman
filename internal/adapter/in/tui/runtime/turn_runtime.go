package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

var errTurnEventsClosed = errors.New("turn event stream closed before completion")

var tuiTurnOwnerSeq atomic.Uint64

func (m *bubbleModel) startTurn(prompt string) tea.Cmd {
	if m.runner == nil {
		m.reconfigureRunner()
	}
	if m.runner == nil {
		slog.DebugContext(m.ctx, "tui turn rejected", "reason", "runner_unavailable")
		m.appendError("model client is not configured; use /model or /provider add to configure")
		m.relayout()
		return nil
	}
	m.retireCompletedTodoForNextTurn()
	m.messages = append(m.messages, model.Message{Role: model.RoleUser, Content: prompt})
	m.retainConversationMessages()
	m.busy = true
	m.busyStarted = time.Now()
	m.turnProgress = turnProgress{}
	m.activeTurnOwner = fmt.Sprintf("tui-turn-%d", tuiTurnOwnerSeq.Add(1))
	m.activity = "analyzing"
	m.historyState.StartThinking()
	m.relayout()
	ctx, cancel := context.WithCancel(m.ctx)
	ctx = agent.WithTurnRef(ctx, agent.TurnRef{SessionID: m.sessionID, TurnID: m.activeTurnOwner})
	m.turnCancel = cancel
	events := make(chan tea.Msg, 32)
	history := model.SnapshotMessages(m.messages)
	startedAt := time.Now()
	slog.DebugContext(ctx, "tui turn started", "prompt_bytes", len(prompt), "history_messages", len(history))
	go func() {
		queueTerminal := func(result app.Result, err error) {
			select {
			case events <- turnDoneMsg{result: result, err: err}:
				slog.DebugContext(ctx, "tui turn terminal message queued")
			case <-m.ctx.Done():
				slog.DebugContext(ctx, "tui turn terminal message dropped", "reason", "ui_context_done")
			}
		}
		defer func() {
			if panicValue := recover(); panicValue != nil {
				stack := debug.Stack()
				slog.DebugContext(ctx, "tui turn worker panicked", "panic_type", fmt.Sprintf("%T", panicValue), "stack_bytes", len(stack))
				queueTerminal(app.Result{}, fmt.Errorf("turn worker panicked: %v", panicValue))
			}
			close(events)
			slog.DebugContext(ctx, "tui turn event channel closed", "duration_ms", time.Since(startedAt).Milliseconds())
		}()
		result, err := m.runner.Run(ctx, history, func(runCtx context.Context, event app.Event) error {
			select {
			case events <- turnDeltaMsg{event: event}:
				return nil
			case <-runCtx.Done():
				return runCtx.Err()
			}
		})
		slog.DebugContext(ctx, "tui turn runner returned", "duration_ms", time.Since(startedAt).Milliseconds(), "success", err == nil, "error_type", errorType(err), "rounds", result.Rounds, "message_count", len(result.Messages))
		queueTerminal(result, err)
	}()
	m.turnEvents = events
	return waitTurnCh(events)
}

func (m *bubbleModel) withSpinner(command tea.Cmd) tea.Cmd {
	if command == nil || !m.busy {
		return command
	}
	return tea.Batch(m.spinner.Tick, command)
}

func waitTurnCh(events <-chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			slog.Debug("tui turn wait observed closed event channel")
			return turnEventsClosedMsg{}
		}
		return msg
	}
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}

func (m *bubbleModel) cancelActiveTurn() int {
	if !m.busy || m.turnCancel == nil {
		return 0
	}
	m.activity = "canceling"
	stopping := 0
	if m.agents.Available() && m.activeTurnOwner != "" {
		stopping = m.agents.CancelTurn(m.activeTurnOwner, agent.CancelTurnAndChildren)
		m.syncAgentSnapshot()
	}
	m.turnCancel()
	m.relayout()
	return stopping
}

func (m *bubbleModel) updateTurnDelta(message turnDeltaMsg) (tea.Model, tea.Cmd) {
	batch := []app.Event{message.event}
	for {
		select {
		case next, ok := <-m.turnEvents:
			if !ok {
				m.applyTurnEvents(batch)
				slog.DebugContext(m.ctx, "tui turn event channel closed before terminal message", "busy", m.busy)
				if m.busy && m.ctx.Err() == nil {
					return m.Update(turnEventsClosedMsg{})
				}
				m.refreshViewport()
				return m, nil
			}
			if delta, isDelta := next.(turnDeltaMsg); isDelta {
				batch = append(batch, delta.event)
				continue
			}
			m.applyTurnEvents(batch)
			// The terminal message performs its own relayout/viewport refresh.
			// Avoid rendering the just-drained deltas twice at turn completion.
			return m.Update(next)
		default:
		}
		break
	}
	m.applyTurnEvents(batch)
	m.refreshViewport()
	return m, m.withSpinner(waitTurnCh(m.turnEvents))
}

func (m *bubbleModel) updateTurnEventsClosed(message turnEventsClosedMsg) (tea.Model, tea.Cmd) {
	slog.DebugContext(m.ctx, "tui turn event channel closed unexpectedly", "busy", m.busy, "context_error", m.ctx.Err() != nil)
	if !m.busy || m.ctx.Err() != nil {
		return m, nil
	}
	return m.Update(turnDoneMsg{err: errTurnEventsClosed})
}

func (m *bubbleModel) updateTurnDone(message turnDoneMsg) (tea.Model, tea.Cmd) {
	slog.DebugContext(m.ctx, "tui turn terminal message received", "success", message.err == nil, "error_type", errorType(message.err), "rounds", message.result.Rounds, "message_count", len(message.result.Messages), "assistant_bytes", len(message.result.Message.Content))
	m.busy = false
	m.busyStarted = time.Time{}
	m.activity = "ready"
	m.turnCancel = nil
	m.turnEvents = nil
	m.activeTurnOwner = ""
	if message.err != nil {
		m.finalizeRunningTools(message.err)
	}
	m.historyState.CommitActive()
	if message.err == nil {
		if len(message.result.Messages) > 0 {
			m.messages = append(m.messages, message.result.Messages...)
		} else if message.result.Message.Content != "" {
			m.messages = append(m.messages, message.result.Message)
		}
	} else if message.err != nil && len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == model.RoleUser {
		messages := m.messages
		messages[len(messages)-1] = model.Message{}
		m.messages = messages[:len(messages)-1]
	}
	m.retainConversationMessages()
	m.appendTurnFailure(message.err)
	m.relayout()
	if message.err != nil {
		m.queue = nil
		return m, nil
	}
	return m, m.withSpinner(m.drainQueue())
}
