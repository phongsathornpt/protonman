package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/session"
)

func generateSessionID(workDir string) string {
	now := time.Now().UTC()
	return fmt.Sprintf("workspace-%s-%s-%03d",
		workspaceKey(workDir),
		now.Format("20060102-150405"),
		now.Nanosecond()/1e6,
	)
}

func resolveSession(
	ctx context.Context,
	store *session.FileStore,
	workDir string,
	options cliOptions,
) (string, session.State, bool, error) {
	wsKey := workspaceKey(workDir)
	wsPrefix := "workspace-" + wsKey

	explicitID := strings.TrimSpace(options.sessionID)
	if explicitID == "" {
		explicitID = strings.TrimSpace(os.Getenv("PROTON_SESSION_ID"))
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

	if explicitID != "" && !options.newSession {
		state, found, err := store.Load(ctx, explicitID)
		if err != nil {
			return "", session.State{}, false, fmt.Errorf("load session %q: %w", explicitID, err)
		}
		return explicitID, state, found, nil
	}

	newID := generateSessionID(workDir)
	return newID, session.State{}, false, nil
}

func workspaceKey(workDir string) string {
	digest := sha256.Sum256([]byte(workDir))
	return hex.EncodeToString(digest[:8])
}
