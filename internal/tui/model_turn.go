package tui

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/model"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
)

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
	m.messages = append(m.messages, model.Message{Role: model.RoleUser, Content: prompt})
	m.busy = true
	m.busyStarted = time.Now()
	m.activity = "thinking"
	m.historyState.SetSpinnerFrame(m.spinner.View())
	m.historyState.StartThinking()
	m.relayout()

	ctx, cancel := context.WithCancel(m.ctx)
	m.turnCancel = cancel
	events := make(chan tea.Msg, 32)
	history := model.CloneMessages(m.messages)
	startedAt := time.Now()
	slog.DebugContext(ctx, "tui turn started",
		"prompt_bytes", len(prompt),
		"history_messages", len(history),
	)
	go func() {
		queueTerminal := func(result applicationturn.Result, err error) {
			select {
			case events <- turnDoneMsg{result: result, err: err}:
				slog.DebugContext(ctx, "tui turn terminal message queued")
			case <-m.ctx.Done():
				slog.DebugContext(ctx, "tui turn terminal message dropped",
					"reason", "ui_context_done",
				)
			}
		}
		defer func() {
			if panicValue := recover(); panicValue != nil {
				stack := debug.Stack()
				slog.DebugContext(ctx, "tui turn worker panicked",
					"panic_type", fmt.Sprintf("%T", panicValue),
					"stack_bytes", len(stack),
				)
				queueTerminal(
					applicationturn.Result{},
					fmt.Errorf("turn worker panicked: %v", panicValue),
				)
			}
			close(events)
			slog.DebugContext(ctx, "tui turn event channel closed",
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)
		}()
		result, err := m.runner.Run(
			ctx,
			history,
			func(runCtx context.Context, event applicationturn.Event) error {
				select {
				case events <- turnDeltaMsg{event: event}:
					return nil
				case <-runCtx.Done():
					return runCtx.Err()
				}
			},
		)
		slog.DebugContext(ctx, "tui turn runner returned",
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"success", err == nil,
			"error_type", errorType(err),
			"rounds", result.Rounds,
			"message_count", len(result.Messages),
		)
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
