package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/sessionfs"
	agenttool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/agent"
	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin"
	skilltool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/skill"
	todotool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/todo"
	webtool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/web"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/base/contextutil"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	"github.com/phongsathornpt/protonman/internal/platform/checkpoint"
	"github.com/phongsathornpt/protonman/internal/platform/sandbox"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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
	layout, err := appdirs.ResolveRuntimeLayout("", "")
	if err != nil {
		return nil, err
	}
	dirs := layout.User
	homeDir := dirs.Home
	workDir := layout.Workspace
	loadedConfig, err := config.Load(ctx, config.Options{HomeDir: homeDir, WorkDir: workDir, ProjectTrusted: envconfig.Bool(envconfig.TrustProject), ProjectScope: &layout.Project})
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	originalSelection := loadedConfig.Model
	reconciled, selectionChanged := config.ReconcileModelSelection(originalSelection, loadedConfig.Providers)
	loadedConfig.Model, loadedConfig.Providers = app.ResolvePrimaryModelDefaults(reconciled, loadedConfig.Providers)
	if selectionChanged {
		loadedConfig.Provenance[config.FieldModelProvider] = config.SourceDefault
		loadedConfig.Provenance[config.FieldModelDefault] = config.SourceDefault
		fmt.Fprintf(os.Stderr, "warning: saved model provider %q is unavailable; using provider %q\n", originalSelection.Provider, loadedConfig.Model.Provider)
	} else {
		if strings.TrimSpace(originalSelection.Provider) == "" && loadedConfig.Model.Provider != "" {
			loadedConfig.Provenance[config.FieldModelProvider] = config.SourceDefault
		}
		if strings.TrimSpace(originalSelection.Default) == "" && loadedConfig.Model.Default != "" {
			loadedConfig.Provenance[config.FieldModelDefault] = config.SourceDefault
		}
	}
	for _, warning := range loadedConfig.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
	workspaceRoot, err := workspace.New(workDir, loadedConfig.ProtectedPaths)
	if err != nil {
		return nil, fmt.Errorf("create workspace policy: %w", err)
	}
	if err := workspaceRoot.ReserveInternalPath(dirs.Root); err != nil {
		return nil, fmt.Errorf("reserve Protonman internal state: %w", err)
	}
	checkpointStore, err := checkpoint.NewWorkspaceFileStore(dirs.Checkpoints, workspaceKey(workDir), workspaceRoot)
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

	skillsResult, err := skill.Discover(ctx, skill.Options{HomeDir: homeDir, WorkDir: workDir, ProjectTrusted: envconfig.Bool(envconfig.TrustProject), ProjectScope: &layout.Project})
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
	stateStore, err := sessionfs.NewFileStore(dirs.Sessions)
	if err != nil {
		return nil, fmt.Errorf("create session store: %w", err)
	}
	observer, err := configuredTelemetryObserver()
	if err != nil {
		return nil, err
	}
	type agentTelemetryObserver interface {
		ObserveAgent(context.Context, string, string, string, string)
		ObserveAgentMetric(context.Context, string, string, string, string, int64, int)
	}
	agentTelemetry, _ := observer.(agentTelemetryObserver)
	sessionID, state, found, err := resolveSession(ctx, stateStore, workDir, options)
	if err != nil {
		return nil, err
	}
	var coordinator *agent.Coordinator
	coordinator = agent.NewCoordinator(nil, nil, workspaceRoot, policy,
		agent.WithLifecycleEventStore(stateStore),
		agent.WithToolRuntimePolicy(loadedConfig.Runtime.ToolPermissionTimeout, loadedConfig.Runtime.ToolExecutionTimeout, observer),
		agent.WithEnabled(loadedConfig.Agent.SubagentsEnabled),
		agent.WithMaxToolCalls(loadedConfig.Agent.MaxToolCalls),
		agent.WithReasoningEffort(loadedConfig.Agent.ReasoningEffort),
		agent.WithSkillRegistry(skillRegistry),
		agent.WithMaxRuntime(loadedConfig.Agent.SubagentMaxRuntime),
		agent.WithDefaultWaitTimeout(loadedConfig.Agent.SubagentWaitTimeout),
		agent.WithDefaultQueueTimeout(loadedConfig.Agent.SubagentQueueTimeout),
		agent.WithMaxLiveAgents(loadedConfig.Agent.MaxLiveSubagents),
		agent.WithMaxRetainedAgents(loadedConfig.Agent.MaxRetainedSubagents),
		agent.WithResultTTL(loadedConfig.Agent.CompletedResultTTL),
		agent.WithMetricObserver(func(metricCtx context.Context, ev agent.MetricEvent) {
			if agentTelemetry != nil {
				agentTelemetry.ObserveAgentMetric(metricCtx, string(ev.Kind), ev.AgentID, ev.ParentID, string(ev.Profile), ev.Bytes, ev.Count)
			}
		}),
		agent.WithEventSink(func(eventCtx context.Context, ev agent.Event) error {
			slog.Debug("subagent lifecycle event", "kind", ev.Kind, "agent_id", ev.AgentID, "parent_id", ev.ParentID, "profile", ev.Profile, "duration", ev.Duration, "err", ev.Err)
			if ev.Kind == agent.EventAgentProgress || coordinator == nil {
				return nil
			}
			ownerSession := strings.TrimSpace(ev.SessionID)
			if ownerSession == "" {
				ownerSession = sessionID
			}
			persistCtx, done := contextutil.DetachedTimeout(eventCtx, runtimepolicy.SessionPersistenceTimeout)
			defer done()
			var persistErr error
			if ev.Kind == agent.EventAgentCompleted || ev.Kind == agent.EventAgentFailed {
				persistErr = coordinator.CompactLifecycleSession(persistCtx, ownerSession)
			}
			if persistErr != nil {
				slog.Warn("persist subagent lifecycle state", "session_id", ownerSession, "error", persistErr)
				if agentTelemetry != nil {
					agentTelemetry.ObserveAgent(persistCtx, "agent_persistence_failure", ev.AgentID, ev.ParentID, string(ev.Profile))
				}
			}
			return nil
		}),
	)
	persistedAgents, agentsFound, loadErr := stateStore.LoadAgents(ctx, sessionID)
	if loadErr != nil {
		return nil, fmt.Errorf("load session subagents: %w", loadErr)
	}
	lifecycleEvents, eventsErr := stateStore.LoadLifecycleEvents(ctx, sessionID)
	if eventsErr != nil {
		return nil, fmt.Errorf("load session subagent lifecycle events: %w", eventsErr)
	}
	if agentsFound || len(lifecycleEvents) > 0 {
		var snapshot *agent.PersistentSnapshot
		if agentsFound {
			snapshot = &persistedAgents
		}
		if recoverErr := coordinator.RecoverLifecycle(ctx, sessionID, snapshot, lifecycleEvents); recoverErr != nil {
			return nil, fmt.Errorf("recover session subagents: %w", recoverErr)
		}
		persistCtx, persistDone := contextutil.DetachedTimeout(ctx, runtimepolicy.SessionPersistenceTimeout)
		if persistErr := coordinator.CompactLifecycleSession(persistCtx, sessionID); persistErr != nil {
			persistDone()
			return nil, fmt.Errorf("compact recovered session subagents: %w", persistErr)
		}
		persistDone()
	}
	failed := true
	defer func() {
		if failed {
			_ = coordinator.Close()
		}
	}()
	subagentModelResolver, err := app.BuildSubagentModelResolver(app.SubagentModelResolverSpec{
		Providers:      loadedConfig.Providers,
		Overrides:      loadedConfig.Agent.Subagents,
		SessionID:      sessionID,
		RequestTimeout: loadedConfig.Runtime.ModelRequestTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("configure subagent models: %w", err)
	}
	coordinator.SetModelResolver(subagentModelResolver)
	subagentReasoningResolver, err := app.BuildSubagentReasoningResolver(loadedConfig.Agent.Subagents)
	if err != nil {
		return nil, fmt.Errorf("configure subagent reasoning: %w", err)
	}
	coordinator.SetReasoningResolver(subagentReasoningResolver)
	resources, err := session.ResolveResources(dirs.Sessions, sessionID)
	if err != nil {
		return nil, fmt.Errorf("resolve session resources: %w", err)
	}
	todoStore, err := tododomain.OpenMarkdownStore(ctx, resources.Todo)
	if err != nil {
		return nil, fmt.Errorf("open session todo store: %w", err)
	}
	baseRegistry, err := builtin.NewDefaultRegistry(workspaceRoot,
		builtin.WithCheckpointStore(checkpointStore),
		builtin.WithSandbox(launcher),
		builtin.WithAdditionalHandlers(
			webtool.NewWebFetch(sandboxProfile.Network, webtool.WithWebFetchTimeout(loadedConfig.Runtime.WebFetchTimeout)),
			todotool.NewTodoForSession(todoStore, sessionID),
			skilltool.NewActivateSkill(skillRegistry, workspaceRoot),
			agenttool.NewSubagent(coordinator),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create tool registry: %w", err)
	}
	var registry tool.Registry = agenttool.NewCapabilityRegistry(baseRegistry, coordinator)
	coordinator.SetParentRegistry(registry)
	initialMode := loadedConfig.Mode
	if found {
		initialMode, err = permission.ParseMode(state.PermissionMode)
		if err != nil {
			return nil, fmt.Errorf("restore session %q: %w", sessionID, err)
		}
		for _, name := range state.ActiveSkills {
			if activateErr := skillRegistry.Activate(name); activateErr != nil {
				return nil, fmt.Errorf("restore active skill %q: %w", name, activateErr)
			}
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
	coordinator.SetPermissionMode(initialMode)
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
	if observer != nil {
		serviceOptions = append(serviceOptions, toolcall.WithObserver(observer))
	}
	service, err := toolcall.NewService(registry, policy, serviceOptions...)
	if err != nil {
		return nil, fmt.Errorf("create tool-call service: %w", err)
	}
	providerKey := strings.ToLower(strings.TrimSpace(loadedConfig.Model.Provider))
	provider := loadedConfig.Providers[providerKey]
	initialRunner, _ := app.BuildConversation(service, skillRegistry, app.NewAgentsForSession(coordinator, sessionID), app.ConversationSpec{
		ProviderName: providerKey, ProviderType: provider.Type, BaseURL: provider.BaseURL, APIKey: provider.APIKey,
		ModelID: loadedConfig.Model.Default, SessionID: sessionID, Workspace: workDir, WorkspacePolicy: workspaceRoot, AgentProfile: loadedConfig.Agent.Profile,
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
	return tool.NewOverlayRegistry(r.registry, todotool.NewTodoForSession(store, sessionID))
}
