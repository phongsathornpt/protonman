// Command proton starts the Proton coding-agent TUI or a headless run.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/projectTHORN/proton/internal/acp"
	"github.com/projectTHORN/proton/internal/app"
	"github.com/projectTHORN/proton/internal/base/envconfig"
	"github.com/projectTHORN/proton/internal/session"
	"github.com/projectTHORN/proton/internal/telemetry"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/tui"
	sdk "github.com/projectTHORN/proton/proton-sdk"
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

	restoreDebugLogger, err := telemetry.ConfigureDebugLogger(envconfig.Value(envconfig.DebugLog))
	if err != nil {
		return fmt.Errorf("configure debug logging: %w", err)
	}
	defer restoreDebugLogger()
	slog.DebugContext(ctx, "proton debug logging enabled")

	runtimeState, err := buildRuntime(ctx, options)
	if err != nil {
		return err
	}
	defer runtimeState.Close()

	if options.acp {
		server, serverErr := acp.New(runtimeState.service, runtimeState.registry, runtimeState.runner, acp.WithSessions(app.NewSessions(runtimeState.stateStore)))
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
		return runHeadless(ctx, runtimeState.service, runtimeState.registry, runtimeState.skills, runtimeState.stateStore, runtimeState.sessionID, runtimeState.state, headlessPrompt, options.output, runtimeState.runner)
	}
	if !stdinIsTerminal() || !stdoutIsTerminal() {
		return fmt.Errorf("refusing to start the TUI without a terminal; use -p, --headless, or --acp")
	}

	bubbleUI, uiErr := tui.NewBubbleTea(
		runtimeState.service,
		runtimeState.registry,
		runtimeState.todoStore,
		tui.WithWorkDir(runtimeState.workDir),
		tui.WithSessionID(runtimeState.sessionID),
		tui.WithSessions(app.NewSessions(runtimeState.stateStore), workspaceKey(runtimeState.workDir)),
		tui.WithInitialMessages(session.ToModelMessages(runtimeState.state.Messages)),
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
	var activeSkills []string
	if runtimeState.skills != nil {
		activeSkills = runtimeState.skills.ActivatedList()
	}
	saveErr := runtimeState.stateStore.Save(ctx, runtimeState.sessionID, session.State{
		SessionID:       runtimeState.sessionID,
		WorkspaceKey:    runtimeState.state.WorkspaceKey,
		WorkspaceName:   runtimeState.state.WorkspaceName,
		CreatedAt:       runtimeState.state.CreatedAt,
		PermissionMode:  runtimeState.service.Mode().String(),
		ActiveSkills:    activeSkills,
		AgentProfile:    bubbleUI.AgentProfile(),
		ReasoningEffort: reasoningSetting(bubbleUI.ReasoningEffort()),
		Messages:        session.FromModelMessages(bubbleUI.SessionState()),
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
		return nil, fmt.Errorf("unsupported PROTON_TELEMETRY value %q", envconfig.Value(envconfig.Telemetry))
	}
}

func reasoningSetting(effort sdk.ReasoningEffort) string {
	if effort == sdk.ReasoningDefault {
		return "auto"
	}
	return string(effort)
}
