package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/agentprompt"
	"github.com/projectTHORN/proton/internal/appdirs"
	"github.com/projectTHORN/proton/internal/checkpoint"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/envconfig"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/sandbox"
	"github.com/projectTHORN/proton/internal/session"
	"github.com/projectTHORN/proton/internal/skill"
	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/tool/builtin"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
	"github.com/projectTHORN/proton/internal/workspace"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

type appRuntime struct {
	workDir     string
	config      config.Snapshot
	coordinator *agent.Coordinator
	todoStore   tododomain.Repository
	registry    tool.Registry
	stateStore  *session.FileStore
	sessionID   string
	state       session.State
	service     *toolcall.Service
	skills      *skill.Registry
	runner      turn.Runner
}

func (r *appRuntime) Close() {
	if r != nil && r.coordinator != nil {
		_ = r.coordinator.Close()
	}
}

func buildRuntime(ctx context.Context, options cliOptions) (*appRuntime, error) {
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return nil, err
	}
	homeDir := dirs.Home
	workDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve work directory: %w", err)
	}
	loadedConfig, err := config.Load(ctx, config.Options{HomeDir: homeDir, WorkDir: workDir, ProjectTrusted: envconfig.Bool(envconfig.TrustProject)})
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	for _, warning := range loadedConfig.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
	workspaceRoot, err := workspace.New(workDir, loadedConfig.ProtectedPaths)
	if err != nil {
		return nil, fmt.Errorf("create workspace policy: %w", err)
	}
	checkpointStore, err := checkpoint.NewFileStore(filepath.Join(dirs.Checkpoints, "workspace-"+workspaceKey(workDir)), workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("create checkpoint store: %w", err)
	}
	sandboxName := loadedConfig.Sandbox
	if configured := strings.TrimSpace(options.sandbox); configured != "" {
		sandboxName, err = sandbox.ParseName(configured)
	} else if configured := envconfig.Value(envconfig.Sandbox); configured != "" {
		sandboxName, err = sandbox.ParseName(configured)
	}
	if err != nil {
		return nil, err
	}
	sandboxProfile, err := sandbox.NewProfile(sandboxName, workDir)
	if err != nil {
		return nil, fmt.Errorf("create sandbox profile: %w", err)
	}
	launcher := sandbox.NewOSLauncher(sandboxProfile)

	skillsResult, err := skill.Discover(ctx, skill.Options{HomeDir: homeDir, WorkDir: workDir, ProjectTrusted: envconfig.Bool(envconfig.TrustProject)})
	if err != nil {
		return nil, fmt.Errorf("discover agent skills: %w", err)
	}
	for _, warning := range skillsResult.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
	skillRegistry := skill.NewRegistry(skillsResult.Skills...)
	policy, err := permission.NewPolicy(loadedConfig.Permission)
	if err != nil {
		return nil, fmt.Errorf("create permission policy: %w", err)
	}
	coordinator := agent.NewCoordinator(nil, nil, workspaceRoot, policy,
		agent.WithMaxToolCalls(loadedConfig.Agent.MaxToolCalls),
		agent.WithReasoningEffort(loadedConfig.Agent.ReasoningEffort),
		agent.WithSkillRegistry(skillRegistry),
		agent.WithMaxRuntime(loadedConfig.Agent.SubagentMaxRuntime),
		agent.WithDefaultWaitTimeout(loadedConfig.Agent.SubagentWaitTimeout),
		agent.WithDefaultQueueTimeout(loadedConfig.Agent.SubagentQueueTimeout),
		agent.WithMaxLiveAgents(loadedConfig.Agent.MaxLiveSubagents),
		agent.WithMaxRetainedAgents(loadedConfig.Agent.MaxRetainedSubagents),
		agent.WithResultTTL(loadedConfig.Agent.CompletedResultTTL),
		agent.WithEventSink(func(ctx context.Context, ev agent.Event) error {
			slog.Debug("subagent lifecycle event", "kind", ev.Kind, "agent_id", ev.AgentID, "parent_id", ev.ParentID, "profile", ev.Profile, "duration", ev.Duration, "err", ev.Err)
			return nil
		}),
	)
	failed := true
	defer func() {
		if failed {
			_ = coordinator.Close()
		}
	}()
	todoStore, err := tododomain.OpenMarkdownStore(ctx, filepath.Join(workDir, tododomain.DefaultFilename))
	if err != nil {
		return nil, fmt.Errorf("open todo store: %w", err)
	}
	registry, err := builtin.NewDefaultRegistry(workspaceRoot,
		builtin.WithCheckpointStore(checkpointStore),
		builtin.WithSandbox(launcher, sandboxProfile.Network),
		builtin.WithSkillRegistry(skillRegistry),
		builtin.WithAgentCoordinator(coordinator),
		builtin.WithTodoStore(todoStore),
		builtin.WithDefaultWebFetchTimeout(loadedConfig.Runtime.WebFetchTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("create tool registry: %w", err)
	}
	coordinator.SetParentRegistry(registry)
	stateStore, err := session.NewFileStore(dirs.Sessions)
	if err != nil {
		return nil, fmt.Errorf("create session store: %w", err)
	}
	sessionID, state, found, err := resolveSession(ctx, stateStore, workDir, options)
	if err != nil {
		return nil, err
	}
	initialMode := loadedConfig.Mode
	if found {
		initialMode, err = permission.ParseMode(state.PermissionMode)
		if err != nil {
			return nil, fmt.Errorf("restore session %q: %w", sessionID, err)
		}
		for _, name := range state.ActiveSkills {
			skillRegistry.MarkActivated(name)
		}
	}
	if options.yolo {
		initialMode = permission.ModeAlwaysApprove
	} else if strings.TrimSpace(options.mode) != "" {
		initialMode, err = permission.ParseMode(options.mode)
		if err != nil {
			return nil, err
		}
	}
	if err := applyAgentProfile(&loadedConfig, &state, options.agentProfile); err != nil {
		return nil, err
	}
	if found && strings.TrimSpace(state.ReasoningEffort) != "" {
		effort, parseErr := sdk.ParseReasoningEffort(state.ReasoningEffort)
		if parseErr != nil {
			return nil, fmt.Errorf("restore session %q reasoning effort: %w", sessionID, parseErr)
		}
		loadedConfig.Agent.ReasoningEffort = effort
	}
	coordinator.SetReasoningEffort(loadedConfig.Agent.ReasoningEffort)
	serviceOptions := []toolcall.Option{
		toolcall.WithMode(initialMode),
		toolcall.WithPermissionTimeout(loadedConfig.Runtime.ToolPermissionTimeout),
		toolcall.WithExecutionTimeout(loadedConfig.Runtime.ToolExecutionTimeout),
		toolcall.WithWorkspaceMutationGate(workspaceRoot),
	}
	observer, err := configuredTelemetryObserver()
	if err != nil {
		return nil, err
	}
	if observer != nil {
		serviceOptions = append(serviceOptions, toolcall.WithObserver(observer))
	}
	service, err := toolcall.NewService(registry, policy, serviceOptions...)
	if err != nil {
		return nil, fmt.Errorf("create tool-call service: %w", err)
	}
	initialRunner := buildInitialRunner(loadedConfig, sessionID, workDir, skillRegistry, coordinator, service)
	failed = false
	return &appRuntime{workDir: workDir, config: loadedConfig, coordinator: coordinator, todoStore: todoStore, registry: registry, stateStore: stateStore, sessionID: sessionID, state: state, service: service, skills: skillRegistry, runner: initialRunner}, nil
}

func applyAgentProfile(loadedConfig *config.Snapshot, state *session.State, requested string) error {
	effectiveProfile := strings.TrimSpace(requested)
	if effectiveProfile == "" && state != nil {
		effectiveProfile = strings.TrimSpace(state.AgentProfile)
	}
	if effectiveProfile == "" {
		effectiveProfile = strings.TrimSpace(loadedConfig.Agent.Profile)
	}
	if effectiveProfile == "" {
		return nil
	}
	prof, err := agent.ParseProfile(effectiveProfile)
	if err != nil {
		return err
	}
	loadedConfig.Agent.Profile = string(prof)
	if state != nil {
		state.AgentProfile = string(prof)
	}
	return nil
}

func buildInitialRunner(cfg config.Snapshot, sessionID, workDir string, skills *skill.Registry, coordinator *agent.Coordinator, service *toolcall.Service) turn.Runner {
	if cfg.Model.Default == "" {
		return nil
	}
	providerKey := strings.ToLower(cfg.Model.Provider)
	if providerKey == "" {
		providerKey = model.DefaultProtonmanName
	}
	provider, ok := cfg.Providers[providerKey]
	if !ok || !model.ProviderHasUsableAuth(providerKey, provider.BaseURL, provider.APIKey) {
		return nil
	}
	languageModel := model.NewProviderLanguageModel(providerKey, provider.Type, provider.BaseURL, provider.APIKey, cfg.Model.Default, model.WithSessionID(sessionID), model.WithRequestTimeout(cfg.Runtime.ModelRequestTimeout))
	coordinator.SetLanguageModel(languageModel)
	promptSpec := agentprompt.Spec{Workspace: workDir}
	if profileName := strings.TrimSpace(cfg.Agent.Profile); profileName != "" {
		if profile, err := agent.ParseProfile(profileName); err == nil {
			promptSpec.Profile = string(profile)
		}
	}
	loopOptions := []turn.Option{
		turn.WithSystemPromptSpec(promptSpec),
		turn.WithMaxToolCalls(cfg.Agent.MaxToolCalls),
		turn.WithTurnTimeout(cfg.Runtime.TurnTimeout),
		turn.WithRoundTimeout(cfg.Runtime.RoundTimeout),
	}
	if profileName := strings.TrimSpace(cfg.Agent.Profile); profileName != "" {
		if profile, err := agent.ParseProfile(profileName); err == nil {
			if spec, ok := agent.SpecForProfile(profile); ok {
				loopOptions = append(loopOptions,
					turn.WithGroundingEvidence(spec.GroundingEvidence),
					turn.WithReasoningEffort(spec.Reasoning),
				)
			}
		}
	}
	if cfg.Agent.ReasoningEffort != sdk.ReasoningDefault {
		loopOptions = append(loopOptions, turn.WithExplicitReasoningEffort(cfg.Agent.ReasoningEffort))
	}
	if skills != nil {
		loopOptions = append(loopOptions, turn.WithSkillRegistry(skills))
	}
	loop, err := turn.NewLoop(languageModel, service, loopOptions...)
	if err != nil {
		return nil
	}
	return loop
}
