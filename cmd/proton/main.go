// Command proton starts the Proton coding-agent bootstrap UI.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	checkpointadapter "github.com/projectTHORN/proton/internal/adapters/checkpoint"
	"github.com/projectTHORN/proton/internal/adapters/config"
	"github.com/projectTHORN/proton/internal/adapters/session"
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

	reader := bufio.NewReader(os.Stdin)
	fullscreen := strings.EqualFold(strings.TrimSpace(os.Getenv("PROTON_TUI")), "fullscreen")
	serviceOptions := []toolcall.Option{toolcall.WithMode(initialMode)}
	if !fullscreen {
		serviceOptions = append(
			serviceOptions,
			toolcall.WithPrompt(tui.NewPermissionPrompt(reader, os.Stdout)),
		)
	}
	service, err := toolcall.NewService(
		registry,
		policy,
		serviceOptions...,
	)
	if err != nil {
		return fmt.Errorf("create tool-call service: %w", err)
	}

	var runErr error
	if fullscreen {
		keys, keyErr := tui.NewRuneKeySource(os.Stdin)
		if keyErr != nil {
			return fmt.Errorf("create full-screen key source: %w", keyErr)
		}
		screen, screenErr := tui.NewANSIScreen(os.Stdout, 0, 0)
		if screenErr != nil {
			return fmt.Errorf("create full-screen screen: %w", screenErr)
		}
		fullScreenUI, uiErr := tui.NewFullScreen(
			service,
			registry,
			keys,
			screen,
			tui.WithTodoItems(loadTodoItems(workDir)),
		)
		if uiErr != nil {
			return fmt.Errorf("create full-screen UI: %w", uiErr)
		}
		service.SetPrompt(fullScreenUI.PermissionPrompt)
		runErr = fullScreenUI.Run(ctx)
	} else {
		ui, uiErr := tui.New(service, registry, reader, os.Stdout)
		if uiErr != nil {
			return fmt.Errorf("create terminal UI: %w", uiErr)
		}
		runErr = ui.Run(ctx)
	}
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
