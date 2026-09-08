package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/phongsathornpt/proton/internal/base/envconfig"
	"github.com/phongsathornpt/proton/internal/core/session"
)

func generateSessionID(workDir string) string { return session.NewID(workDir) }

func resolveSession(
	ctx context.Context,
	store session.Repository,
	workDir string,
	options cliOptions,
) (string, session.State, bool, error) {
	wsKey := workspaceKey(workDir)
	wsPrefix := "workspace-" + wsKey
	workspaceName := filepath.Base(filepath.Clean(workDir))

	cliID := strings.TrimSpace(options.sessionID)
	envID := ""
	if cliID == "" {
		envID = envconfig.Value(envconfig.SessionID)
	}
	explicitID := cliID
	if explicitID == "" {
		explicitID = envID
	}

	if options.resume {
		if explicitID != "" {
			state, found, err := store.Load(ctx, explicitID)
			if err != nil {
				return "", session.State{}, false, fmt.Errorf("load session %q: %w", explicitID, err)
			}
			if !found {
				return "", session.State{}, false, fmt.Errorf("session %q not found to resume", explicitID)
			}
			if state.WorkspaceKey != "" && state.WorkspaceKey != wsKey {
				return "", session.State{}, false, fmt.Errorf("session %q belongs to another workspace", explicitID)
			}
			return explicitID, state, true, nil
		}

		latestID, latestState, found, err := store.LatestSession(ctx, wsPrefix)
		if err != nil {
			return "", session.State{}, false, fmt.Errorf("find latest session: %w", err)
		}
		if !found {
			return "", session.State{}, false, fmt.Errorf("no previous session found for workspace")
		}
		return latestID, latestState, true, nil
	}

	if cliID != "" {
		_, found, err := store.Load(ctx, cliID)
		if err != nil {
			return "", session.State{}, false, fmt.Errorf("check session %q: %w", cliID, err)
		}
		if found {
			return "", session.State{}, false, fmt.Errorf("session %q already exists; use --resume --session %s", cliID, cliID)
		}
		return cliID, session.State{SessionID: cliID, WorkspaceKey: wsKey, WorkspaceName: workspaceName}, false, nil
	}

	if envID != "" && !options.newSession {
		state, found, err := store.Load(ctx, envID)
		if err != nil {
			return "", session.State{}, false, fmt.Errorf("load session %q: %w", envID, err)
		}
		if found {
			if state.WorkspaceKey != "" && state.WorkspaceKey != wsKey {
				return "", session.State{}, false, fmt.Errorf("session %q belongs to another workspace", envID)
			}
			return envID, state, true, nil
		}
		return envID, session.State{SessionID: envID, WorkspaceKey: wsKey, WorkspaceName: workspaceName}, false, nil
	}

	newID := generateSessionID(workDir)
	return newID, session.State{SessionID: newID, WorkspaceKey: wsKey, WorkspaceName: workspaceName}, false, nil
}

func workspaceKey(workDir string) string { return session.WorkspaceKey(workDir) }
