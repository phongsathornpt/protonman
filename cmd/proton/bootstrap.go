package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/projectTHORN/proton/internal/adapter/out/config"
	"github.com/projectTHORN/proton/internal/adapter/out/model"
	"github.com/projectTHORN/proton/internal/adapter/out/sessionfs"
	agenttool "github.com/projectTHORN/proton/internal/adapter/out/tool/agent"
	"github.com/projectTHORN/proton/internal/adapter/out/tool/builtin"
	skilltool "github.com/projectTHORN/proton/internal/adapter/out/tool/skill"
	todotool "github.com/projectTHORN/proton/internal/adapter/out/tool/todo"
	webtool "github.com/projectTHORN/proton/internal/adapter/out/tool/web"
	"github.com/projectTHORN/proton/internal/app"
	"github.com/projectTHORN/proton/internal/app/appdirs"
	"github.com/projectTHORN/proton/internal/base/envconfig"
	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/session"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/core/workspace"
	"github.com/projectTHORN/proton/internal/engine/toolcall"
	"github.com/projectTHORN/proton/internal/feature/agent"
	"github.com/projectTHORN/proton/internal/feature/skill"
	tododomain "github.com/projectTHORN/proton/internal/feature/todo"
	"github.com/projectTHORN/proton/internal/platform/checkpoint"
	"github.com/projectTHORN/proton/internal/platform/sandbox"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

type appRuntime struct {
	workDir      string
	config       config.Snapshot
	coordinator  *agent.Coordinator
	todoStore    tododomain.Repository
	registry     tool.Registry
	stateStore   session.Repository
	sessionsRoot string
	sessionID    string
	state        session.State
	service      *toolcall.Service
	skills       *skill.Registry
	runner       app.Conversation
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
	stateStore, err := sessionfs.NewFileStore(dirs.Sessions)
	if err != nil {
		return nil, fmt.Errorf("create session store: %w", err)
	}
	sessionID, state, found, err := resolveSession(ctx, stateStore, workDir, options)
	if err != nil {
		return nil, err
	}
	resources, err := session.ResolveResources(dirs.Sessions, sessionID)
	if err != nil {
		return nil, fmt.Errorf("resolve session resources: %w", err)
	}
	todoStore, err := tododomain.OpenMarkdownStore(ctx, resources.Todo)
	if err != nil {
		return nil, fmt.Errorf("open session todo store: %w", err)
	}
	registry, err := builtin.NewDefaultRegistry(workspaceRoot,
		builtin.WithCheckpointStore(checkpointStore),
		builtin.WithSandbox(launcher),
		builtin.WithAdditionalHandlers(
			webtool.NewWebFetch(sandboxProfile.Network, webtool.WithWebFetchTimeout(loadedConfig.Runtime.WebFetchTimeout)),
			todotool.NewGetTodoForSession(todoStore, sessionID),
			todotool.NewUpdateTodoForSession(todoStore, sessionID),
			skilltool.NewActivateSkill(skillRegistry, workspaceRoot),
			agenttool.NewDelegateTask(coordinator),
			agenttool.NewWaitAgent(coordinator),
			agenttool.NewGetAgent(coordinator),
			agenttool.NewListAgents(coordinator),
			agenttool.NewCancelAgent(coordinator),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create tool registry: %w", err)
	}
	coordinator.SetParentRegistry(registry)
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
	providerKey := strings.ToLower(strings.TrimSpace(loadedConfig.Model.Provider))
	if providerKey == "" {
		providerKey = model.DefaultProtonmanName
	}
	provider := loadedConfig.Providers[providerKey]
	initialRunner, _ := app.BuildConversation(service, skillRegistry, app.NewAgents(coordinator), app.ConversationSpec{
		ProviderName: providerKey, ProviderType: provider.Type, BaseURL: provider.BaseURL, APIKey: provider.APIKey,
		ModelID: loadedConfig.Model.Default, SessionID: sessionID, Workspace: workDir, AgentProfile: loadedConfig.Agent.Profile,
		ReasoningEffort: loadedConfig.Agent.ReasoningEffort, MaxToolCalls: loadedConfig.Agent.MaxToolCalls,
		RequestTimeout: loadedConfig.Runtime.ModelRequestTimeout, TurnTimeout: loadedConfig.Runtime.TurnTimeout, RoundTimeout: loadedConfig.Runtime.RoundTimeout,
	})
	failed = false
	return &appRuntime{workDir: workDir, config: loadedConfig, coordinator: coordinator, todoStore: todoStore, registry: registry, stateStore: stateStore, sessionsRoot: dirs.Sessions, sessionID: sessionID, state: state, service: service, skills: skillRegistry, runner: initialRunner}, nil
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

func (r *appRuntime) registryForSession(sessionID string) (tool.Registry, error) {
	if r == nil || r.registry == nil {
		return nil, fmt.Errorf("base tool registry is unavailable")
	}
	resources, err := session.ResolveResources(r.sessionsRoot, sessionID)
	if err != nil {
		return nil, err
	}
	store, err := tododomain.OpenMarkdownStore(context.Background(), resources.Todo)
	if err != nil {
		return nil, err
	}
	return tool.NewOverlayRegistry(r.registry, todotool.NewGetTodoForSession(store, sessionID), todotool.NewUpdateTodoForSession(store, sessionID))
}
