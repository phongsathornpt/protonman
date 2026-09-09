package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	tea "charm.land/bubbletea/v2"

	crashview "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/crash"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/projectpolicy"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

func (ui *BubbleTeaUI) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		slog.DebugContext(ctx, "tui run rejected",
			"reason", "context_already_done",
			"error_type", fmt.Sprintf("%T", err),
		)
		return fmt.Errorf("start Bubble Tea UI: %w", err)
	}
	startedAt := time.Now()
	slog.DebugContext(ctx, "tui run started",
		"initial_messages", len(ui.initialMessages),
		"has_runner", ui.runner != nil,
	)
	ui.service.SetPrompt(ui.PermissionPrompt)
	defer ui.service.SetCallGuard(nil)
	ui.agents.SetPrompt(ui.PermissionPrompt)
	ui.agents.SetPermissionMode(ui.service.Mode())
	defer ui.agents.SetCallGuard(nil)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer ui.bridge.Close()

	currentMessages := model.SnapshotMessages(ui.initialMessages)
	agentRuntime := newAgentRuntimeState(ui.agentConfig, ui.hasAgentConfig)

	for {
		todoSnapshot := tododomain.Snapshot{}
		if ui.todoStore != nil {
			todoSnapshot = ui.todoStore.Snapshot()
		}
		todoItems := todoSnapshot.Items
		bModel := newBubbleModel(
			runCtx,
			ui.service,
			ui.registry,
			todoItems,
			ui.runner,
			ui.bridge,
			ui.workDir,
			currentMessages,
		)
		bModel.agents = ui.agents
		cancelAgentEvents := func() {}
		if ui.agents.Available() {
			bModel.agentEvents, cancelAgentEvents = ui.agents.Subscribe(32)
			bModel.agentSnapshot = ui.agents.List()
		}
		bModel.todoStore = ui.todoStore
		bModel.todoRevision = todoSnapshot.Revision
		bModel.skills = ui.skills
		bModel.activeModel = ui.modelConfig.Default
		bModel.activeProvider = ui.modelConfig.Provider
		bModel.providers = ui.providers
		bModel.sessionID = ui.sessionID
		bModel.sessions = ui.sessions
		bModel.workspaceKey = ui.workspaceKey
		bModel.projectTrusted = ui.projectTrusted
		bModel.projectConfigSources = append([]string(nil), ui.projectConfigSources...)
		bModel.projectConfigProvenance = projectpolicy.CloneProvenance(ui.projectConfigProvenance)
		if ui.hasRuntimeConfig {
			bModel.runtimeConfig = ui.runtimeConfig
		}
		agentRuntime.apply(bModel)
		bModel.reconfigureRunner()

		program := tea.NewProgram(
			bModel,
			tea.WithContext(runCtx),
		)

		var finalModel tea.Model
		var err error
		var panicVal any
		var panicStack []byte

		func() {
			defer func() {
				if r := recover(); r != nil {
					panicVal = r
					panicStack = debug.Stack()
				}
			}()
			finalModel, err = program.Run()
		}()
		cancelAgentEvents()

		// Bubble model updates mutate the same runtime state object. Capture the
		// latest controls even when Bubble Tea exits through the crash screen so a
		// restart cannot silently restore stale config defaults.
		agentRuntime.capture(bModel)
		ui.modelConfig.Default = bModel.activeModel
		ui.modelConfig.Provider = bModel.activeProvider
		ui.finalAgentProfile = bModel.agentProfile
		ui.finalReasoningEffort = bModel.reasoningEffort

		if panicVal != nil {
			slog.DebugContext(ctx, "tui program panicked",
				"error_type", fmt.Sprintf("%T", panicVal),
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)
			crash := crashview.NewCrashModel(panicVal, panicStack)
			crashProg := tea.NewProgram(
				crash,
				tea.WithContext(runCtx),
			)
			finalCrash, _ := crashProg.Run()
			if cm, ok := finalCrash.(*crashview.CrashModel); ok && cm.RestartRequested() {
				slog.DebugContext(ctx, "tui crash screen requested restart")
				continue
			}
			slog.DebugContext(ctx, "tui run stopped after panic")
			return fmt.Errorf("protonman crashed: %v", panicVal)
		}

		if modelState, ok := finalModel.(*bubbleModel); ok {
			ui.finalMessages = model.SnapshotMessages(modelState.messages)
			ui.finalAgentProfile = modelState.agentProfile
			ui.finalReasoningEffort = modelState.reasoningEffort
			currentMessages = model.SnapshotMessages(modelState.messages)
			slog.DebugContext(ctx, "tui program returned",
				"duration_ms", time.Since(startedAt).Milliseconds(),
				"message_count", len(modelState.messages),
				"busy", modelState.busy,
			)
		} else {
			slog.DebugContext(ctx, "tui program returned without bubble model")
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				slog.DebugContext(ctx, "tui run stopped by context",
					"error_type", fmt.Sprintf("%T", ctxErr),
				)
				return fmt.Errorf("run Bubble Tea UI: %w", ctxErr)
			}
			slog.DebugContext(ctx, "tui run failed",
				"error_type", fmt.Sprintf("%T", err),
			)
			return fmt.Errorf("run Bubble Tea UI: %w", err)
		}
		slog.DebugContext(ctx, "tui run completed",
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
		return nil
	}
}
