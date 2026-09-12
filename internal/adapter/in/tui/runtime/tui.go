// Package runtime provides the Bubble Tea fullscreen terminal adapter for Protonman.
package runtime

import (
	"context"
	"errors"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/permissionbridge"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// BubbleTeaUI is the Bubble Tea terminal adapter over Protonman services.
type BubbleTeaUI struct {
	service                 *toolcall.Service
	registry                tool.Registry
	skills                  *skill.Registry
	todoStore               tododomain.Repository
	runner                  app.Conversation
	application             app.Services
	bridge                  *permissionbridge.Bridge
	agents                  app.Agents
	workDir                 string
	initialMessages         []model.Message
	activeGoal              string
	finalActiveGoal         string
	finalMessages           []model.Message
	finalAgentProfile       string
	finalReasoningEffort    sdk.ReasoningEffort
	lowConcurrencyMode      model.LowConcurrencySetting
	modelConfig             config.ModelConfig
	agentConfig             config.AgentConfig
	hasAgentConfig          bool
	runtimeConfig           config.RuntimeConfig
	hasRuntimeConfig        bool
	providers               map[string]config.ProviderConfig
	sessionID               string
	sessions                *app.Sessions
	workspaceKey            string
	projectTrusted          bool
	projectConfigSources    []string
	projectConfigProvenance map[string]config.ValueSource
}

// NewBubbleTea creates the component-based fullscreen TUI.
func NewBubbleTea(
	service *toolcall.Service,
	registry tool.Registry,
	todoStore tododomain.Repository,
	options ...BubbleTeaOption,
) (*BubbleTeaUI, error) {
	if service == nil {
		return nil, errors.New("Bubble Tea UI service is required")
	}
	if registry == nil {
		return nil, errors.New("Bubble Tea UI registry is required")
	}
	ui := &BubbleTeaUI{
		service:   service,
		registry:  registry,
		todoStore: todoStore,
		bridge:    permissionbridge.New(),
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(ui); err != nil {
			return nil, err
		}
	}
	ui.finalMessages = model.SnapshotMessages(ui.initialMessages)
	ui.finalActiveGoal = ui.activeGoal
	ui.finalAgentProfile = ui.agentConfig.Profile
	ui.finalReasoningEffort = ui.agentConfig.ReasoningEffort
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

// ActiveGoal returns the latest persistent objective selected by the TUI.
func (ui *BubbleTeaUI) ActiveGoal() string {
	return ui.finalActiveGoal
}

// AgentProfile returns the latest named profile selected by the TUI.
func (ui *BubbleTeaUI) AgentProfile() string {
	return ui.finalAgentProfile
}

// ReasoningEffort returns the latest explicit session reasoning override.
func (ui *BubbleTeaUI) ReasoningEffort() sdk.ReasoningEffort {
	return ui.finalReasoningEffort
}

// LowConcurrencyMode returns the latest session-local low-concurrency override.
func (ui *BubbleTeaUI) LowConcurrencyMode() string {
	return ui.lowConcurrencyMode.String()
}

// Run starts Bubble Tea with raw input, alternate-screen rendering, and mouse
// cell motion. Bubble Tea owns terminal restoration even on program failure.
