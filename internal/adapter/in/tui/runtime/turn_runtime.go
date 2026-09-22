package runtime

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/runtimeui"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"github.com/phongsathornpt/protonman/internal/feature/imageprep"
)

var errTurnEventsClosed = errors.New("turn event stream closed before completion")

var tuiTurnOwnerSeq atomic.Uint64

type imageSubmissionPreparedMsg struct {
	preparationID uint64
	input         tuiconv.QueuedInput
	message       model.Message
	err           error
}

func prepareImageSubmission(preparationID uint64, input tuiconv.QueuedInput) tea.Cmd {
	input = input.Clone()
	return func() tea.Msg {
		parts := make([]model.ContentPart, 0, len(input.Attachments)+1)
		for _, attachment := range input.Attachments {
			snapshot, err := imageprep.SnapshotLocal(attachment.Path)
			if err != nil {
				return imageSubmissionPreparedMsg{
					preparationID: preparationID,
					input:         input,
					err:           fmt.Errorf("prepare %s: %w", attachment.Placeholder, err),
				}
			}
			parts = append(parts, model.ContentPart{
				Type:     model.ContentPartImage,
				MIMEType: snapshot.MIMEType,
				Data:     base64.StdEncoding.EncodeToString(snapshot.Bytes),
			})
		}
		if text := strings.TrimSpace(input.Text); text != "" {
			parts = append(parts, model.ContentPart{Type: model.ContentPartText, Text: text})
		}
		return imageSubmissionPreparedMsg{
			preparationID: preparationID,
			input:         input,
			message: model.Message{
				ID:      model.NewMessageID(),
				Role:    model.RoleUser,
				Content: strings.TrimSpace(input.Text),
				Parts:   parts,
			},
		}
	}
}

func (m *bubbleModel) beginImagePreparation(input tuiconv.QueuedInput) tea.Cmd {
	m.imagePreparationID++
	preparationID := m.imagePreparationID
	pending := input.Clone()
	m.pendingImageInput = &pending
	m.imagePreparing = true
	m.activity = "preparing image"
	m.requestRelayout()
	return prepareImageSubmission(preparationID, input)
}

func (m *bubbleModel) updateImageSubmissionPrepared(message imageSubmissionPreparedMsg) tea.Cmd {
	if message.preparationID != m.imagePreparationID {
		return nil
	}
	m.imagePreparing = false
	m.pendingImageInput = nil
	m.activity = runtimeui.ActivityReady
	if message.err != nil {
		m.appendError(message.err.Error())
		m.restoreSubmissionToComposer(message.input)
		m.requestRelayout()
		return nil
	}
	cleanupQueuedInputAttachments(message.input)
	if history := submissionHistoryText(message.input); history != "" {
		m.panes.bottom.recordHistory(history)
	}
	m.appendUser(submissionDisplayText(message.input))
	return m.startTurnMessage(message.message)
}

func (m *bubbleModel) cancelImagePreparation() bool {
	if m == nil || !m.imagePreparing {
		return false
	}
	m.imagePreparationID++
	m.imagePreparing = false
	m.activity = runtimeui.ActivityReady
	var pending *tuiconv.QueuedInput
	if m.pendingImageInput != nil {
		clone := m.pendingImageInput.Clone()
		pending = &clone
	}
	m.pendingImageInput = nil
	if pending != nil {
		m.restoreCanceledImageSubmission(*pending)
	}
	m.requestRelayout()
	return true
}

func (m *bubbleModel) restoreCanceledImageSubmission(input tuiconv.QueuedInput) {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		cleanupQueuedInputAttachments(input)
		return
	}
	prompt := m.panes.bottom.prompt()
	if strings.TrimSpace(prompt.Value()) != "" || len(m.panes.bottom.composer.attachments.localImages) > 0 {
		cleanupQueuedInputAttachments(input)
		return
	}
	m.restoreSubmissionToComposer(input)
}

func (m *bubbleModel) restoreSubmissionToComposer(input tuiconv.QueuedInput) {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		cleanupQueuedInputAttachments(input)
		return
	}
	prompt := m.panes.bottom.prompt()
	if strings.TrimSpace(prompt.Value()) != "" || len(m.panes.bottom.composer.attachments.localImages) > 0 {
		if m.conversation == nil || !m.conversation.EnqueueInput(input) {
			cleanupQueuedInputAttachments(input)
		}
		return
	}
	prompt.SetValue(input.Text)
	prompt.CursorEnd()
	for _, attachment := range input.Attachments {
		if attachment.Temporary {
			m.panes.bottom.composer.attachments.attachTemporaryImage(prompt, attachment.Path)
			continue
		}
		m.panes.bottom.attachImage(attachment.Path)
	}
	m.panes.bottom.syncPromptChrome()
}

func (m *bubbleModel) startTurn(prompt string) tea.Cmd {
	return m.startTurnMessage(model.Message{
		ID:      model.NewMessageID(),
		Role:    model.RoleUser,
		Content: prompt,
	})
}

func (m *bubbleModel) startTurnMessage(userMessage model.Message) tea.Cmd {
	if m.busy {
		m.appendError("a turn is already running")
		m.requestRelayout()
		return nil
	}
	if m.runner == nil {
		m.reconfigureRunner()
	}
	if m.runner == nil {
		slog.DebugContext(m.ctx, "tui turn rejected", "reason", "runner_unavailable")
		m.appendError("model client is not configured; use /model or /provider add to configure")
		m.requestRelayout()
		return nil
	}
	if userMessage.ID == "" {
		userMessage.ID = model.NewMessageID()
	}
	if userMessage.Role == "" {
		userMessage.Role = model.RoleUser
	}
	if userMessage.Role != model.RoleUser {
		m.appendError("tui turn input must be a user message")
		m.requestRelayout()
		return nil
	}
	m.retireCompletedTodoForNextTurn()
	m.conversation.AppendMessages(userMessage)
	m.turnModelState.beginTurn(fmt.Sprintf("tui-turn-%d", tuiTurnOwnerSeq.Add(1)), time.Now())
	m.requestRelayout()
	ctx, cancel := context.WithCancel(m.ctx)
	ctx = agent.WithTurnRef(ctx, agent.TurnRef{SessionID: m.sessionID, TurnID: m.activeTurnOwner})
	events := make(chan tea.Msg, 32)
	history := m.conversation.SnapshotMessages()
	startedAt := time.Now()
	slog.DebugContext(ctx, "tui turn started", "prompt_bytes", len(userMessage.Content), "content_parts", len(userMessage.Parts), "history_messages", len(history))
	// Capture model-owned references on the Update thread before spawning:
	// the worker must never read mutable fields, because runner
	// reconfiguration can replace m.runner while this turn is still finishing.
	runner := m.runner
	uiCtx := m.ctx
	go func() {
		queueTerminal := func(result app.Result, err error) {
			select {
			case events <- turnmsg.Done{Result: result, Err: err}:
				slog.DebugContext(ctx, "tui turn terminal message queued")
			case <-uiCtx.Done():
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
		result, err := runner.Run(ctx, history, func(runCtx context.Context, event app.Event) error {
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
	m.turnModelState.bindTurn(cancel, events)
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
	m.activity = runtimeui.ActivityCanceling
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
	m.turnModelState.finishTurn()
	if message.Err != nil {
		m.finalizeRunningTools(message.Err)
	}
	m.historyState.CommitActive()
	m.historyState.FinalizeRetryingTools()
	if len(message.Result.Messages) > 0 && (message.Err == nil || message.Result.ReplaySafe) {
		m.conversation.AppendMessages(message.Result.Messages...)
	} else if message.Err == nil && message.Result.Message.Content != "" {
		m.conversation.AppendMessages(message.Result.Message)
	} else if message.Err != nil {
		m.conversation.DropTrailingUserMessage()
	}
	m.appendTurnFailure(message.Err)
	if message.Err == nil && message.Result.GoalCompleted && m.activeGoal != "" {
		if err := m.setActiveGoal(""); err != nil {
			m.appendError("complete active goal: " + err.Error())
		} else {
			m.appendMuted("goal · completed")
			m.retireCompletedTodoForNextTurn()
		}
	}
	m.requestRelayout()
	if message.Err != nil {
		m.clearQueuedInputs()
		return nil
	}
	return m.withSpinner(m.drainQueue())
}

func (s *turnModelState) beginTurn(owner string, started time.Time) {
	s.turnProgress = turnProgress{}
	s.activeTurnOwner = owner
	s.busy = true
	s.busyStarted = started
	s.activity = ""
}

func (s *turnModelState) bindTurn(cancel context.CancelFunc, events <-chan tea.Msg) {
	s.turnCancel = cancel
	s.turnEvents = events
}

func (s *turnModelState) finishTurn() {
	s.busy = false
	s.busyStarted = time.Time{}
	s.activity = runtimeui.ActivityReady
	s.turnCancel = nil
	s.turnEvents = nil
	s.activeTurnOwner = ""
}

func (s *turnModelState) beginTool(activity string, started time.Time, cancel context.CancelFunc) {
	s.turnProgress = turnProgress{}
	s.activeTurnOwner = ""
	s.busy = true
	s.busyStarted = started
	s.activity = activity
	s.turnCancel = cancel
	s.turnEvents = nil
}

func (s *turnModelState) finishTool() {
	s.finishTurn()
	s.turnProgress = turnProgress{}
}
