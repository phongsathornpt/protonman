// Command proton starts the Proton coding-agent TUI or a headless run.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/projectTHORN/proton/internal/acp"
	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/checkpoint"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/headless"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/sandbox"
	"github.com/projectTHORN/proton/internal/session"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/telemetry"
	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/tool/builtin"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/tui"
	"github.com/projectTHORN/proton/internal/turn"
	"github.com/projectTHORN/proton/internal/workspace"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	options, err := parseArgs(args)
	if err != nil {
		return fmt.Errorf("%v\n\n%s", err, usage())
	}
	if options.help {
		fmt.Fprint(os.Stdout, usage())
		return nil
	}

	restoreDebugLogger, err := telemetry.ConfigureDebugLogger(os.Getenv("PROTON_DEBUG_LOG"))
	if err != nil {
		return fmt.Errorf("configure debug logging: %w", err)
	}
	defer restoreDebugLogger()
	slog.DebugContext(ctx, "proton debug logging enabled")

	homeDir := strings.TrimSpace(os.Getenv("PROTON_HOME"))
	if homeDir == "" {
		resolvedHome, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = resolvedHome
	}
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve work directory: %w", err)
	}

	loadedConfig, err := config.Load(ctx, config.Options{
		HomeDir:        homeDir,
		WorkDir:        workDir,
		ProjectTrusted: truthy(os.Getenv("PROTON_TRUST_PROJECT")),
	})
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	for _, warning := range loadedConfig.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}

	workspaceRoot, err := workspace.New(workDir, loadedConfig.ProtectedPaths)
	if err != nil {
		return fmt.Errorf("create workspace policy: %w", err)
	}
	checkpointRoot := filepath.Join(
		homeDir,
		".proton",
		"checkpoints",
		"workspace-"+workspaceKey(workDir),
	)
	checkpointStore, err := checkpoint.NewFileStore(checkpointRoot, workspaceRoot)
	if err != nil {
		return fmt.Errorf("create checkpoint store: %w", err)
	}

	sandboxName := loadedConfig.Sandbox
	if configured := strings.TrimSpace(options.sandbox); configured != "" {
		sandboxName, err = sandbox.ParseName(configured)
		if err != nil {
			return err
		}
	} else if configured := strings.TrimSpace(os.Getenv("PROTON_SANDBOX")); configured != "" {
		sandboxName, err = sandbox.ParseName(configured)
		if err != nil {
			return err
		}
	}
	sandboxProfile, err := sandbox.NewProfile(sandboxName, workDir)
	if err != nil {
		return fmt.Errorf("create sandbox profile: %w", err)
	}
	// Always build an explicit launcher, even for the Off profile.
	// Off yields a bare shell via OSLauncher (explicit opt-out), never a nil
	// launcher, so bash/git_status fail-closed guards stay satisfied.
	launcher := sandbox.NewOSLauncher(sandboxProfile)

	skillsResult, err := skill.Discover(ctx, skill.Options{
		HomeDir:        homeDir,
		WorkDir:        workDir,
		ProjectTrusted: truthy(os.Getenv("PROTON_TRUST_PROJECT")),
	})
	if err != nil {
		return fmt.Errorf("discover agent skills: %w", err)
	}
	for _, warning := range skillsResult.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
	skillRegistry := skill.NewRegistry(skillsResult.Skills...)

	policy, err := permission.NewPolicy(loadedConfig.Permission)
	if err != nil {
		return fmt.Errorf("create permission policy: %w", err)
	}

	coordinator := agent.NewCoordinator(nil, nil, workspaceRoot, policy,
		agent.WithMaxRounds(loadedConfig.Agent.MaxRounds),
		agent.WithMaxToolCalls(loadedConfig.Agent.MaxToolCalls),
		agent.WithMaxRuntime(loadedConfig.Agent.SubagentMaxRuntime),
		agent.WithDefaultWaitTimeout(loadedConfig.Agent.SubagentWaitTimeout),
		agent.WithDefaultQueueTimeout(loadedConfig.Agent.SubagentQueueTimeout),
		agent.WithMaxLiveAgents(loadedConfig.Agent.MaxLiveSubagents),
		agent.WithMaxRetainedAgents(loadedConfig.Agent.MaxRetainedSubagents),
		agent.WithResultTTL(loadedConfig.Agent.CompletedResultTTL),
		agent.WithEventSink(func(ctx context.Context, ev agent.Event) error {
			slog.Debug("subagent lifecycle event",
				"kind", ev.Kind,
				"agent_id", ev.AgentID,
				"parent_id", ev.ParentID,
				"profile", ev.Profile,
				"duration", ev.Duration,
				"err", ev.Err,
			)
			return nil
		}),
	)
	defer func() { _ = coordinator.Close() }()

	todoStore, err := tododomain.OpenMarkdownStore(ctx, filepath.Join(workDir, "TODO.md"))
	if err != nil {
		return fmt.Errorf("open todo store: %w", err)
	}

	registry, err := builtin.NewDefaultRegistry(
		workspaceRoot,
		builtin.WithCheckpointStore(checkpointStore),
		builtin.WithSandbox(launcher, sandboxProfile.Network),
		builtin.WithSkillRegistry(skillRegistry),
		builtin.WithAgentCoordinator(coordinator),
		builtin.WithTodoStore(todoStore),
	)
	if err != nil {
		return fmt.Errorf("create tool registry: %w", err)
	}
	coordinator.SetParentRegistry(registry)

	stateStore, err := session.NewFileStore(filepath.Join(homeDir, ".proton", "sessions"))
	if err != nil {
		return fmt.Errorf("create session store: %w", err)
	}
	sessionID, state, found, err := resolveSession(ctx, stateStore, workDir, options)
	if err != nil {
		return err
	}
	initialMode := loadedConfig.Mode
	if found {
		initialMode, err = permission.ParseMode(state.PermissionMode)
		if err != nil {
			return fmt.Errorf("restore session %q: %w", sessionID, err)
		}
		if skillRegistry != nil {
			for _, name := range state.ActiveSkills {
				skillRegistry.MarkActivated(name)
			}
		}
	}
	if options.yolo {
		initialMode = permission.ModeAlwaysApprove
	} else if strings.TrimSpace(options.mode) != "" {
		initialMode, err = permission.ParseMode(options.mode)
		if err != nil {
			return err
		}
	}

	effectiveProfile := strings.TrimSpace(options.agentProfile)
	if effectiveProfile == "" {
		effectiveProfile = loadedConfig.Agent.Profile
	}
	if effectiveProfile != "" {
		prof, profErr := agent.ParseProfile(effectiveProfile)
		if profErr != nil {
			return profErr
		}
		loadedConfig.Agent.Profile = string(prof)
		promptContent := agent.SystemPromptForProfile(prof)
		if len(state.Messages) == 0 {
			state.Messages = []session.Message{
				{Role: model.RoleSystem, Content: promptContent},
			}
		} else if state.Messages[0].Role != model.RoleSystem {
			state.Messages = append([]session.Message{
				{Role: model.RoleSystem, Content: promptContent},
			}, state.Messages...)
		} else if len(state.Messages) == 1 && state.Messages[0].Role == model.RoleSystem {
			state.Messages[0].Content = promptContent
		}
	}

	serviceOptions := []toolcall.Option{toolcall.WithMode(initialMode)}
	observer, observerErr := configuredTelemetryObserver()
	if observerErr != nil {
		return observerErr
	}
	if observer != nil {
		serviceOptions = append(serviceOptions, toolcall.WithObserver(observer))
	}
	service, err := toolcall.NewService(
		registry,
		policy,
		serviceOptions...,
	)
	if err != nil {
		return fmt.Errorf("create tool-call service: %w", err)
	}

	var initialRunner turn.Runner
	if loadedConfig.Model.Default != "" {
		provKey := strings.ToLower(loadedConfig.Model.Provider)
		if provKey == "" {
			provKey = model.DefaultProtonmanName
		}
		if prov, ok := loadedConfig.Providers[provKey]; ok && strings.TrimSpace(prov.APIKey) != "" {
			baseURL := model.ResolveProviderBaseURL(provKey, prov.BaseURL)
			client := model.NewProviderClient(provKey, baseURL, prov.APIKey, loadedConfig.Model.Default, model.WithSessionID(sessionID))
			coordinator.SetClient(client)
			var loopOpts []turn.Option
			if skillRegistry != nil {
				loopOpts = append(loopOpts, turn.WithSkillRegistry(skillRegistry))
			}
			loopOpts = append(loopOpts, turn.WithMaxRounds(loadedConfig.Agent.MaxRounds))
			loopOpts = append(loopOpts, turn.WithMaxToolCalls(loadedConfig.Agent.MaxToolCalls))
			loop, loopErr := turn.NewLoop(client, service, loopOpts...)
			if loopErr == nil {
				initialRunner = loop
			}
		}
	}

	if options.acp {
		server, serverErr := acp.New(service, registry, initialRunner, acp.WithStore(stateStore))
		if serverErr != nil {
			return fmt.Errorf("create ACP server: %w", serverErr)
		}
		return server.Serve(ctx, os.Stdin, os.Stdout)
	}
	headlessPrompt := strings.TrimSpace(options.prompt)
	if options.headless && headlessPrompt == "" {
		headlessPrompt, err = readStdinPrompt()
		if err != nil {
			return err
		}
	}
	if headlessPrompt != "" {
		return runHeadless(ctx, service, registry, skillRegistry, stateStore, sessionID, state, headlessPrompt, options.output, initialRunner)
	}
	if !stdinIsTerminal() || !stdoutIsTerminal() {
		return fmt.Errorf("refusing to start the TUI without a terminal; use -p, --headless, or --acp")
	}

	bubbleUI, uiErr := tui.NewBubbleTea(
		service,
		registry,
		todoStore,
		tui.WithWorkDir(workDir),
		tui.WithSessionID(sessionID),
		tui.WithInitialMessages(session.ToModelMessages(state.Messages)),
		tui.WithSkills(skillRegistry),
		tui.WithModelConfig(loadedConfig.Model, loadedConfig.Providers),
		tui.WithAgentConfig(loadedConfig.Agent),
		tui.WithBubbleTeaRunner(initialRunner),
		tui.WithCoordinator(coordinator),
	)
	if uiErr != nil {
		return fmt.Errorf("create Bubble Tea UI: %w", uiErr)
	}
	runErr := bubbleUI.Run(ctx)
	var activeSkills []string
	if skillRegistry != nil {
		activeSkills = skillRegistry.ActivatedList()
	}
	saveErr := stateStore.Save(ctx, sessionID, session.State{
		PermissionMode: service.Mode().String(),
		ActiveSkills:   activeSkills,
		Messages:       session.FromModelMessages(bubbleUI.SessionState()),
	})
	if runErr != nil && saveErr != nil {
		return fmt.Errorf("run terminal UI: %v; save session: %w", runErr, saveErr)
	}
	if runErr != nil {
		return fmt.Errorf("run terminal UI: %w", runErr)
	}
	if saveErr != nil {
		return fmt.Errorf("save session: %w", saveErr)
	}
	return nil
}

func runHeadless(
	ctx context.Context,
	service *toolcall.Service,
	registry tool.Registry,
	skillRegistry *skill.Registry,
	stateStore *session.FileStore,
	sessionID string,
	state session.State,
	prompt string,
	outputFormat string,
	turnRunner turn.Runner,
) error {
	format, err := headless.ParseFormat(outputFormat)
	if err != nil {
		return err
	}
	runner, err := headless.New(service, registry, turnRunner, headless.WithSkills(skillRegistry))
	if err != nil {
		return fmt.Errorf("create headless runner: %w", err)
	}
	if err := runner.LoadSession(state); err != nil {
		return fmt.Errorf("restore session transcript: %w", err)
	}
	runErr := runner.Run(ctx, prompt, os.Stdout, format)
	var activeSkills []string
	if skillRegistry != nil {
		activeSkills = skillRegistry.ActivatedList()
	}
	saveErr := stateStore.Save(ctx, sessionID, session.State{
		PermissionMode: service.Mode().String(),
		ActiveSkills:   activeSkills,
		Messages:       runner.SessionState(),
	})
	if runErr != nil && saveErr != nil {
		return fmt.Errorf("run headless: %v; save session: %w", runErr, saveErr)
	}
	if runErr != nil {
		return fmt.Errorf("run headless: %w", runErr)
	}
	if saveErr != nil {
		return fmt.Errorf("save session: %w", saveErr)
	}
	return nil
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func configuredTelemetryObserver() (toolcall.Observer, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PROTON_TELEMETRY"))) {
	case "", "off", "false", "0":
		return nil, nil
	case "stderr":
		logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
		observer, err := telemetry.NewSlogObserver(logger)
		if err != nil {
			return nil, fmt.Errorf("create telemetry observer: %w", err)
		}
		return observer, nil
	default:
		return nil, fmt.Errorf("unsupported PROTON_TELEMETRY value %q", os.Getenv("PROTON_TELEMETRY"))
	}
}
