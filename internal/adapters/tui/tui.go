// Package tui provides the Bubble Tea fullscreen terminal adapter for Proton.
package tui

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/application/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/application/turn"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
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

// WithWorkDir sets the workspace path shown on the welcome card.
func WithWorkDir(dir string) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.workDir = dir
		return nil
	}
}

// BubbleTeaUI is the Bubble Tea terminal adapter over Proton services.
type BubbleTeaUI struct {
	service  *toolcall.Service
	registry tool.Registry
	todo     []TodoItem
	runner   applicationturn.Runner
	bridge   *permissionBridge
	workDir  string
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

// Run starts Bubble Tea with raw input, alternate-screen rendering, and mouse
// cell motion. Bubble Tea owns terminal restoration even on program failure.
func (ui *BubbleTeaUI) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start Bubble Tea UI: %w", err)
	}
	ui.service.SetPrompt(ui.PermissionPrompt)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer ui.bridge.Close()

	program := tea.NewProgram(
		newBubbleModel(runCtx, ui.service, ui.registry, ui.todo, ui.runner, ui.bridge, ui.workDir),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithContext(runCtx),
	)
	if _, err := program.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("run Bubble Tea UI: %w", ctxErr)
		}
		return fmt.Errorf("run Bubble Tea UI: %w", err)
	}
	return nil
}
