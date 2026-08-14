// Command proton starts the Proton coding-agent bootstrap UI.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	checkpointadapter "github.com/projectTHORN/proton/internal/adapters/checkpoint"
	"github.com/projectTHORN/proton/internal/adapters/config"
	"github.com/projectTHORN/proton/internal/adapters/session"
	"github.com/projectTHORN/proton/internal/adapters/telemetry"
	"github.com/projectTHORN/proton/internal/adapters/tools"
	"github.com/projectTHORN/proton/internal/adapters/tui"
	"github.com/projectTHORN/proton/internal/adapters/workspace"
	"github.com/projectTHORN/proton/internal/application/toolcall"
	"github.com/projectTHORN/proton/internal/domain/permission"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
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
	checkpointStore, err := checkpointadapter.NewFileStore(checkpointRoot, workspaceRoot)
	if err != nil {
		return fmt.Errorf("create checkpoint store: %w", err)
	}

	registry, err := tools.NewDefaultRegistry(workspaceRoot, checkpointStore)
	if err != nil {
		return fmt.Errorf("create tool registry: %w", err)
	}

	policy, err := permission.NewPolicy(loadedConfig.Permission)
	if err != nil {
		return fmt.Errorf("create permission policy: %w", err)
	}

	sessionID := resolveSessionID(workDir)
	stateStore, err := session.NewFileStore(filepath.Join(homeDir, ".proton", "sessions"))
	if err != nil {
		return fmt.Errorf("create session store: %w", err)
	}
	initialMode := loadedConfig.Mode
	state, found, err := stateStore.Load(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session %q: %w", sessionID, err)
	}
	if found {
		initialMode, err = permission.ParseMode(state.PermissionMode)
		if err != nil {
			return fmt.Errorf("restore session %q: %w", sessionID, err)
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

	bubbleUI, uiErr := tui.NewBubbleTea(
		service,
		registry,
		loadTodoItems(workDir),
	)
	if uiErr != nil {
		return fmt.Errorf("create Bubble Tea UI: %w", uiErr)
	}
	runErr := bubbleUI.Run(ctx)
	saveErr := stateStore.Save(ctx, sessionID, session.State{
		PermissionMode: service.Mode().String(),
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

func loadTodoItems(workDir string) []tui.TodoItem {
	contents, err := os.ReadFile(filepath.Join(workDir, "TODO.md"))
	if err != nil {
		return []tui.TodoItem{}
	}
	return tui.ParseTODO(string(contents))
}

func resolveSessionID(workDir string) string {
	if configured := strings.TrimSpace(os.Getenv("PROTON_SESSION_ID")); configured != "" {
		return configured
	}
	return "workspace-" + workspaceKey(workDir)
}

func workspaceKey(workDir string) string {
	digest := sha256.Sum256([]byte(workDir))
	return hex.EncodeToString(digest[:8])
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
