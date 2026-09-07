package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/envconfig"
	"github.com/projectTHORN/proton/internal/session"
)

func generateSessionID(workDir string) string {
	now := time.Now().UTC()
	var entropy [4]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		binary := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", workDir, now.UnixNano())))
		copy(entropy[:], binary[:4])
	}
	return fmt.Sprintf("workspace-%s-%s-%s",
		workspaceKey(workDir),
		now.Format("20060102-150405.000000000"),
		hex.EncodeToString(entropy[:]),
	)
}

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

func workspaceKey(workDir string) string {
	digest := sha256.Sum256([]byte(workDir))
	return hex.EncodeToString(digest[:8])
}
