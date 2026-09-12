// Command protonman starts the Protonman coding-agent TUI or a headless run.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/phongsathornpt/protonman/internal/adapter/in/acp"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui"
	mcpadapter "github.com/phongsathornpt/protonman/internal/adapter/out/tool/mcp"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/platform/telemetry"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func configureACPMCP(ctx context.Context, cwd string, registry tool.Registry, configs []acp.MCPServerConfig) (io.Closer, error) {
	registrar, ok := registry.(tool.DynamicRegistrar)
	if !ok {
		return nil, fmt.Errorf("session registry does not support dynamic MCP registration")
	}
	servers := make([]mcpadapter.ManagedServer, 0, len(configs))
	for _, config := range configs {
		server, err := mcpadapter.NewStdioServer(config.Name, config.Command, config.Args, config.Env, cwd)
		if err != nil {
			for _, started := range servers {
				_ = started.Close()
			}
			return nil, err
		}
		servers = append(servers, server)
	}
	manager, err := mcpadapter.NewManager(servers...)
	if err != nil {
		for _, server := range servers {
			_ = server.Close()
		}
		return nil, err
	}
	if err := manager.Bind(ctx, registrar); err != nil {
		return nil, err
	}
	return manager, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if handled, err := runSessionCommand(ctx, args); handled {
		return err
	}
	options, err := parseArgs(args)
	if err != nil {
		return fmt.Errorf("%v\n\n%s", err, usage())
	}
	if options.help {
		fmt.Fprint(os.Stdout, usage())
		return nil
	}
	if options.version {
		fmt.Fprintf(os.Stdout, "%s %s\n", buildinfo.Name, buildinfo.Version())
		return nil
	}

	restoreDebugLogger, err := telemetry.ConfigureDebugLogger(envconfig.Value(envconfig.DebugLog))
	if err != nil {
		return fmt.Errorf("configure debug logging: %w", err)
	}
	defer restoreDebugLogger()
	slog.DebugContext(ctx, "protonman debug logging enabled")

	runtimeState, err := buildRuntime(ctx, options)
	if err != nil {
		return err
	}
	defer runtimeState.Close()

	if options.acp {
		server, serverErr := acp.New(
			runtimeState.service, runtimeState.registry, runtimeState.runner,
			acp.WithSessions(app.NewSessions(runtimeState.stateStore)),
			acp.WithAgents(app.NewAgents(runtimeState.coordinator)),
			acp.WithSessionRegistryFactory(func(sessionID, _ string) (tool.Registry, error) {
				return runtimeState.registryForSession(sessionID)
			}),
			acp.WithMCPRegistryConfigurer(configureACPMCP),
		)
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
		return runHeadless(ctx, headlessInvocation{
			service:       runtimeState.service,
			registry:      runtimeState.registry,
			skillRegistry: runtimeState.skills,
			stateStore:    runtimeState.stateStore,
			sessionID:     runtimeState.sessionID,
			state:         runtimeState.state,
			prompt:        headlessPrompt,
			outputFormat:  options.output,
			turnRunner:    runtimeState.runner,
			agents:        app.NewAgents(runtimeState.coordinator),
		})
	}
	if !stdinIsTerminal() || !stdoutIsTerminal() {
		return fmt.Errorf("refusing to start the TUI without a terminal; use -p, --headless, or --acp")
	}

	bubbleUI, uiErr := tui.NewBubbleTea(
		runtimeState.service,
		runtimeState.registry,
		runtimeState.todoStore,
		tui.WithApplicationServices(runtimeState.application),
		tui.WithWorkDir(runtimeState.workDir),
		tui.WithSessionID(runtimeState.sessionID),
		tui.WithSessions(app.NewSessions(runtimeState.stateStore), workspaceKey(runtimeState.workDir)),
		tui.WithInitialMessages(session.ToModelMessages(runtimeState.state.Messages)),
		tui.WithActiveGoal(runtimeState.state.ActiveGoal),
		tui.WithLowConcurrencyMode(runtimeState.state.LowConcurrencyMode),
		tui.WithSkills(runtimeState.skills),
		tui.WithModelConfig(runtimeState.config.Model, runtimeState.config.Providers),
		tui.WithAgentConfig(runtimeState.config.Agent),
		tui.WithRuntimeConfig(runtimeState.config.Runtime),
		tui.WithProjectContext(envconfig.Bool(envconfig.TrustProject), runtimeState.config.Sources, runtimeState.config.Provenance),
		tui.WithBubbleTeaRunner(runtimeState.runner),
		tui.WithCoordinator(runtimeState.coordinator),
	)
	if uiErr != nil {
		return fmt.Errorf("create Bubble Tea UI: %w", uiErr)
	}
	runErr := bubbleUI.Run(ctx)
	activeSkills := []string{}
	if runtimeState.skills != nil {
		activeSkills = runtimeState.skills.ActivatedList()
	}
	saveErr := runtimeState.stateStore.Save(ctx, runtimeState.sessionID, session.State{
		SessionID:          runtimeState.sessionID,
		Revision:           runtimeState.state.Revision,
		WorkspaceKey:       runtimeState.state.WorkspaceKey,
		WorkspaceName:      runtimeState.state.WorkspaceName,
		CreatedAt:          runtimeState.state.CreatedAt,
		PermissionMode:     runtimeState.service.Mode().String(),
		ActiveSkills:       activeSkills,
		ActiveGoal:         bubbleUI.ActiveGoal(),
		AgentProfile:       bubbleUI.AgentProfile(),
		ReasoningEffort:    reasoningSetting(bubbleUI.ReasoningEffort()),
		LowConcurrencyMode: bubbleUI.LowConcurrencyMode(),
		Messages:           session.FromModelMessages(bubbleUI.SessionState()),
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

func truthy(value string) bool { return envconfig.Truthy(value) }

func configuredTelemetryObserver() (toolcall.Observer, error) {
	switch strings.ToLower(envconfig.Value(envconfig.Telemetry)) {
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
		return nil, fmt.Errorf("unsupported %s value %q", envconfig.Telemetry, envconfig.Value(envconfig.Telemetry))
	}
}

func reasoningSetting(effort sdk.ReasoningEffort) string {
	if effort == sdk.ReasoningDefault {
		return "auto"
	}
	return string(effort)
}
