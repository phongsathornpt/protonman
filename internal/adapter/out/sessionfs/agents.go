package sessionfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

const maxAgentSnapshotBytes = 2 * 1024 * 1024

// LoadAgents loads retained subagent lifecycle state owned by one session.
func (s *FileStore) LoadAgents(ctx context.Context, sessionID string) (agent.PersistentSnapshot, bool, error) {
	if err := session.ValidateID(sessionID); err != nil {
		return agent.PersistentSnapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return agent.PersistentSnapshot{}, false, fmt.Errorf("before loading agent snapshot: %w", err)
	}
	resources, err := session.ResolveResources(s.root, sessionID)
	if err != nil {
		return agent.PersistentSnapshot{}, false, err
	}
	file, err := os.Open(resources.Agents)
	if errors.Is(err, os.ErrNotExist) {
		return agent.PersistentSnapshot{}, false, nil
	}
	if err != nil {
		return agent.PersistentSnapshot{}, false, fmt.Errorf("open agent snapshot: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAgentSnapshotBytes+1))
	if err != nil {
		return agent.PersistentSnapshot{}, false, fmt.Errorf("read agent snapshot: %w", err)
	}
	if len(data) > maxAgentSnapshotBytes {
		return agent.PersistentSnapshot{}, false, fmt.Errorf("agent snapshot exceeds %d bytes", maxAgentSnapshotBytes)
	}
	var snapshot agent.PersistentSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return agent.PersistentSnapshot{}, false, fmt.Errorf("decode agent snapshot: %w", err)
	}
	if snapshot.Version != agent.PersistentSnapshotVersion {
		return agent.PersistentSnapshot{}, false, fmt.Errorf("unsupported agent snapshot version %d", snapshot.Version)
	}
	return snapshot, true, nil
}

// SaveAgents atomically stores retained subagent lifecycle state for one session.
func (s *FileStore) SaveAgents(ctx context.Context, sessionID string, snapshot agent.PersistentSnapshot) error {
	if err := session.ValidateID(sessionID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before saving agent snapshot: %w", err)
	}
	return s.withSessionLock(ctx, sessionID, func() error {
		resources, err := session.ResolveResources(s.root, sessionID)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(resources.Root, 0o700); err != nil {
			return fmt.Errorf("create session directory: %w", err)
		}
		if err := os.Chmod(resources.Root, 0o700); err != nil {
			return fmt.Errorf("protect session directory: %w", err)
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("encode agent snapshot: %w", err)
		}
		if len(data) > maxAgentSnapshotBytes {
			return fmt.Errorf("agent snapshot exceeds %d bytes", maxAgentSnapshotBytes)
		}
		return writeAgentSnapshot(ctx, resources.Root, resources.Agents, data)
	})
}

func writeAgentSnapshot(ctx context.Context, root, destination string, data []byte) (writeErr error) {
	file, err := os.CreateTemp(root, ".agents-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary agent snapshot: %w", err)
	}
	temporaryPath := file.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); closeErr != nil && writeErr == nil {
				writeErr = fmt.Errorf("close temporary agent snapshot: %w", closeErr)
			}
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && writeErr == nil {
			writeErr = fmt.Errorf("remove temporary agent snapshot: %w", removeErr)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect temporary agent snapshot: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write agent snapshot: %w", err)
	}
	if _, err := file.Write([]byte("\n")); err != nil {
		return fmt.Errorf("terminate agent snapshot: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync agent snapshot: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close agent snapshot: %w", err)
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before installing agent snapshot: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("install agent snapshot: %w", err)
	}
	return nil
}
