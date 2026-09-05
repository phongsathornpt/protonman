// Package tui provides the Bubble Tea fullscreen terminal adapter for Proton.
package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
)

// BubbleTeaOption configures the Bubble Tea fullscreen adapter.
type BubbleTeaOption func(*BubbleTeaUI) error

// WithBubbleTeaRunner connects ordinary prompt input to the model/tool loop.
func WithBubbleTeaRunner(runner applicationturn.Runner) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.runner = runner
		return nil
	}
}

// WithModelConfig attaches model preferences and provider configurations to the TUI.
func WithModelConfig(modelCfg config.ModelConfig, providers map[string]config.ProviderConfig) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.modelConfig = modelCfg
		if providers != nil {
			ui.providers = make(map[string]config.ProviderConfig, len(providers))
			for k, v := range providers {
				ui.providers[k] = v
			}
		}
		return nil
	}
}

// WithSkills attaches an Agent Skill registry for slash commands and display.
func WithSkills(skills *skill.Registry) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.skills = skills
		return nil
	}
}

// WithWorkDir sets the workspace path shown on the welcome card.
func WithWorkDir(dir string) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.workDir = dir
		return nil
	}
}

// WithInitialMessages restores a previously persisted provider-neutral transcript.
func WithInitialMessages(messages []model.Message) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.initialMessages = model.CloneMessages(messages)
		return nil
	}
}

// WithSessionID configures the active conversation session identifier.
func WithSessionID(sessionID string) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.sessionID = sessionID
		return nil
	}
}

// WithAgentConfig attaches agent execution settings (e.g. max rounds) to the TUI.
func WithAgentConfig(agentCfg config.AgentConfig) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.agentConfig = agentCfg
		ui.hasAgentConfig = true
		return nil
	}
}

// WithCoordinator attaches the subagent coordinator to the TUI so permission
// mode, interactive prompts, and model client changes are synchronized.
func WithCoordinator(coordinator *agent.Coordinator) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.coordinator = coordinator
		return nil
	}
}

// BubbleTeaUI is the Bubble Tea terminal adapter over Proton services.
type BubbleTeaUI struct {
	service         *toolcall.Service
	registry        tool.Registry
	skills          *skill.Registry
	todo            []TodoItem
	runner          applicationturn.Runner
	bridge          *permissionBridge
	coordinator     *agent.Coordinator
	workDir         string
	initialMessages []model.Message
	finalMessages   []model.Message
	modelConfig     config.ModelConfig
	agentConfig     config.AgentConfig
	hasAgentConfig  bool
	providers       map[string]config.ProviderConfig
	sessionID       string
}

// NewBubbleTea creates the component-based fullscreen TUI.
func NewBubbleTea(
	service *toolcall.Service,
	registry tool.Registry,
	todo []TodoItem,
	options ...BubbleTeaOption,
) (*BubbleTeaUI, error) {
	if service == nil {
		return nil, errors.New("Bubble Tea UI service is required")
	}
	if registry == nil {
		return nil, errors.New("Bubble Tea UI registry is required")
	}
	ui := &BubbleTeaUI{
		service:  service,
		registry: registry,
		todo:     append([]TodoItem{}, todo...),
		bridge:   newPermissionBridge(),
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(ui); err != nil {
			return nil, err
		}
	}
	ui.finalMessages = model.CloneMessages(ui.initialMessages)
	return ui, nil
}

// PermissionPrompt adapts a synchronous service permission request into a
// Bubble Tea modal request/response exchange.
func (ui *BubbleTeaUI) PermissionPrompt(
	ctx context.Context,
	request permission.Request,
) (permission.Resolution, error) {
	return ui.bridge.Prompt(ctx, request)
}

// SessionState returns the latest provider-neutral conversation state owned by
// the TUI. The returned slice can be persisted without sharing mutable backing
// storage with the live Bubble Tea model.
func (ui *BubbleTeaUI) SessionState() []model.Message {
	return model.CloneMessages(ui.finalMessages)
}

// Run starts Bubble Tea with raw input, alternate-screen rendering, and mouse
// cell motion. Bubble Tea owns terminal restoration even on program failure.
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
	if ui.coordinator != nil {
		ui.coordinator.SetPrompt(ui.PermissionPrompt)
		ui.coordinator.SetPermissionMode(ui.service.Mode())
		defer ui.coordinator.SetCallGuard(nil)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer ui.bridge.Close()

	currentMessages := model.CloneMessages(ui.initialMessages)

	for {
		bModel := newBubbleModel(
			runCtx,
			ui.service,
			ui.registry,
			ui.todo,
			ui.runner,
			ui.bridge,
			ui.workDir,
			currentMessages,
		)
		bModel.coordinator = ui.coordinator
		bModel.skills = ui.skills
		bModel.activeModel = ui.modelConfig.Default
		bModel.activeProvider = ui.modelConfig.Provider
		bModel.providers = ui.providers
		bModel.sessionID = ui.sessionID
		if ui.hasAgentConfig {
			bModel.maxRounds = ui.agentConfig.MaxRounds
			bModel.maxToolCalls = ui.agentConfig.MaxToolCalls
			bModel.agentProfile = ui.agentConfig.Profile
		}
		bModel.reconfigureRunner()

		program := tea.NewProgram(
			bModel,
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
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

		if panicVal != nil {
			slog.DebugContext(ctx, "tui program panicked",
				"error_type", fmt.Sprintf("%T", panicVal),
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)
			crash := NewCrashModel(panicVal, panicStack)
			crashProg := tea.NewProgram(
				crash,
				tea.WithAltScreen(),
				tea.WithContext(runCtx),
			)
			finalCrash, _ := crashProg.Run()
			if cm, ok := finalCrash.(*CrashModel); ok && cm.restart {
				slog.DebugContext(ctx, "tui crash screen requested restart")
				continue
			}
			slog.DebugContext(ctx, "tui run stopped after panic")
			return fmt.Errorf("proton crashed: %v", panicVal)
		}

		if modelState, ok := finalModel.(*bubbleModel); ok {
			ui.finalMessages = model.CloneMessages(modelState.messages)
			currentMessages = model.CloneMessages(modelState.messages)
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
