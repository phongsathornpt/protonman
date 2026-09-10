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
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
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
		m.requestRelayout()
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
	m.requestRelayout()
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
			case events <- turnmsg.Done{Result: result, Err: err}:
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
			case events <- turnmsg.Delta{Event: event}:
				return nil
			case <-runCtx.Done():
				return runCtx.Err()
			}
		})
		slog.DebugContext(ctx, "tui turn runner returned", "duration_ms", time.Since(startedAt).Milliseconds(), "success", err == nil, "error_type", errorType(err), "rounds", result.Rounds, "message_count", len(result.Messages))
		queueTerminal(result, err)
	}()
	m.turnEvents = events
	return turnmsg.Wait(events)
}

func (m *bubbleModel) withSpinner(command tea.Cmd) tea.Cmd {
	if command == nil || !m.busy {
		return command
	}
	return tea.Batch(m.spinner.Tick, command)
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
	m.requestRelayout()
	return stopping
}

func (m *bubbleModel) updateTurnDelta(message turnmsg.Delta) tea.Cmd {
	batch := []app.Event{message.Event}
	for {
		select {
		case next, ok := <-m.turnEvents:
			if !ok {
				m.applyTurnEvents(batch)
				slog.DebugContext(m.ctx, "tui turn event channel closed before terminal message", "busy", m.busy)
				if m.busy && m.ctx.Err() == nil {
					return m.updateTurnEventsClosed(turnmsg.EventsClosed{})
				}
				m.refreshViewport()
				return nil
			}
			if delta, isDelta := next.(turnmsg.Delta); isDelta {
				batch = append(batch, delta.Event)
				continue
			}
			m.applyTurnEvents(batch)
			switch terminal := next.(type) {
			case turnmsg.Done:
				return m.updateTurnDone(terminal)
			case turnmsg.EventsClosed:
				return m.updateTurnEventsClosed(terminal)
			default:
				slog.DebugContext(m.ctx, "tui turn ignored unexpected event", "event_type", fmt.Sprintf("%T", next))
				m.refreshViewport()
				return m.withSpinner(turnmsg.Wait(m.turnEvents))
			}
		default:
		}
		break
	}
	m.applyTurnEvents(batch)
	m.refreshViewport()
	return m.withSpinner(turnmsg.Wait(m.turnEvents))
}

func (m *bubbleModel) updateTurnEventsClosed(_ turnmsg.EventsClosed) tea.Cmd {
	slog.DebugContext(m.ctx, "tui turn event channel closed unexpectedly", "busy", m.busy, "context_error", m.ctx.Err() != nil)
	if !m.busy || m.ctx.Err() != nil {
		return nil
	}
	return m.updateTurnDone(turnmsg.Done{Err: errTurnEventsClosed})
}

func (m *bubbleModel) updateTurnDone(message turnmsg.Done) tea.Cmd {
	slog.DebugContext(m.ctx, "tui turn terminal message received", "success", message.Err == nil, "error_type", errorType(message.Err), "rounds", message.Result.Rounds, "message_count", len(message.Result.Messages), "assistant_bytes", len(message.Result.Message.Content))
	m.busy = false
	m.busyStarted = time.Time{}
	m.activity = "ready"
	m.turnCancel = nil
	m.turnEvents = nil
	m.activeTurnOwner = ""
	if message.Err != nil {
		m.finalizeRunningTools(message.Err)
	}
	m.historyState.CommitActive()
	if message.Err == nil {
		if len(message.Result.Messages) > 0 {
			m.messages = append(m.messages, message.Result.Messages...)
		} else if message.Result.Message.Content != "" {
			m.messages = append(m.messages, message.Result.Message)
		}
	} else if message.Err != nil && len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == model.RoleUser {
		messages := m.messages
		messages[len(messages)-1] = model.Message{}
		m.messages = messages[:len(messages)-1]
	}
	m.retainConversationMessages()
	m.appendTurnFailure(message.Err)
	m.requestRelayout()
	if message.Err != nil {
		m.queue = nil
		return nil
	}
	return m.withSpinner(m.drainQueue())
}
