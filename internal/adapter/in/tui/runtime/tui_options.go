package runtime

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/projectconfig"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

// BubbleTeaOption configures the Bubble Tea fullscreen adapter.
type BubbleTeaOption func(*BubbleTeaUI) error

// WithApplicationServices attaches application use cases required by the TUI.
func WithApplicationServices(services app.Services) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.application = services
		return nil
	}
}

// WithBubbleTeaRunner connects ordinary prompt input to the model/tool loop.
func WithBubbleTeaRunner(runner app.Conversation) BubbleTeaOption {
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

// WithWorkDir sets the workspace path shown in the session header.
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

// WithActiveGoal restores the persistent objective for the active session.
func WithActiveGoal(goal string) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.activeGoal = strings.TrimSpace(goal)
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

// WithSessionStore attaches session discovery to the TUI without making the UI own persistence.
func WithSessions(sessions *app.Sessions, workspaceKey string) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.sessions = sessions
		ui.workspaceKey = workspaceKey
		return nil
	}
}

// TodoHandlerFactory builds a session-bound task tool handler.
type TodoHandlerFactory func(store tododomain.Repository, sessionID string) tool.Handler

// WithTodoHandlerFactory configures the factory used to rebind the todo tool on session switch.
func WithTodoHandlerFactory(factory TodoHandlerFactory) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.todoHandlerFactory = factory
		return nil
	}
}

// WithAgentConfig attaches agent execution settings to the TUI.
func WithAgentConfig(agentCfg config.AgentConfig) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.agentConfig = agentCfg
		ui.hasAgentConfig = true
		return nil
	}
}

// WithRuntimeConfig attaches shared execution and network policy to the TUI.
func WithRuntimeConfig(runtimeCfg config.RuntimeConfig) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.runtimeConfig = runtimeCfg
		ui.hasRuntimeConfig = true
		return nil
	}
}

// WithProjectContext attaches workspace trust and loaded config-source metadata.
func WithProjectContext(trusted bool, sources []string, provenance map[string]config.ValueSource) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.projectTrusted = trusted
		ui.projectConfigSources = append([]string(nil), sources...)
		ui.projectConfigProvenance = projectconfig.CloneProvenance(provenance)
		return nil
	}
}

// WithLowConcurrencyMode restores the session-local low-concurrency override.
func WithLowConcurrencyMode(raw string) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		setting, err := model.ParseLowConcurrencySetting(raw)
		if err != nil {
			return err
		}
		ui.lowConcurrencyMode = setting
		return nil
	}
}

// WithCoordinator attaches the subagent coordinator to the TUI so permission
// mode, interactive prompts, and model client changes are synchronized.
func WithCoordinator(coordinator *agent.Coordinator) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.agents = app.NewAgentsForSession(coordinator, ui.sessionID)
		return nil
	}
}
