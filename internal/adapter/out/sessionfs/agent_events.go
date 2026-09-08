package sessionfs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

const maxAgentEventJournalBytes = 16 * 1024 * 1024

// AppendLifecycleEvent durably appends one lifecycle fact to a session journal.
func (s *FileStore) AppendLifecycleEvent(ctx context.Context, event agent.LifecycleEvent) error {
	if err := session.ValidateID(event.SessionID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before appending agent lifecycle event: %w", err)
	}
	return s.withSessionLock(ctx, event.SessionID, func() error {
		return s.appendLifecycleEventLocked(ctx, event)
	})
}
func (s *FileStore) appendLifecycleEventLocked(ctx context.Context, event agent.LifecycleEvent) error {
	resources, err := session.ResolveResources(s.root, event.SessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(resources.Root, 0o700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}
	if err := os.Chmod(resources.Root, 0o700); err != nil {
		return fmt.Errorf("protect session directory: %w", err)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode agent lifecycle event: %w", err)
	}
	if len(data)+1 > maxAgentEventJournalBytes {
		return fmt.Errorf("agent lifecycle event exceeds journal limit")
	}
	file, err := os.OpenFile(resources.AgentEvents, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open agent event journal: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect agent event journal: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat agent event journal: %w", err)
	}
	if info.Size()+int64(len(data))+1 > maxAgentEventJournalBytes {
		return fmt.Errorf("agent event journal exceeds %d bytes", maxAgentEventJournalBytes)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before writing agent lifecycle event: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append agent lifecycle event: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync agent event journal: %w", err)
	}
	return nil
}

// LoadLifecycleEvents replays the append-only lifecycle journal for one session.
func (s *FileStore) LoadLifecycleEvents(ctx context.Context, sessionID string) ([]agent.LifecycleEvent, error) {
	if err := session.ValidateID(sessionID); err != nil {
		return nil, err
	}
	resources, err := session.ResolveResources(s.root, sessionID)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(resources.AgentEvents)
	if errors.Is(err, os.ErrNotExist) {
		return []agent.LifecycleEvent{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open agent event journal: %w", err)
	}
	defer file.Close()
	reader := io.LimitReader(file, maxAgentEventJournalBytes+1)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	events := make([]agent.LifecycleEvent, 0)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("replay agent lifecycle events: %w", err)
		}
		var event agent.LifecycleEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode agent lifecycle event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan agent lifecycle events: %w", err)
	}
	if info, err := file.Stat(); err == nil && info.Size() > maxAgentEventJournalBytes {
		return nil, fmt.Errorf("agent event journal exceeds %d bytes", maxAgentEventJournalBytes)
	}
	return events, nil
}

// CompactLifecycle atomically installs the supplied session projection before
// truncating lifecycle facts already represented by it. A crash before truncation
// only leaves replay duplicates, which aggregate versions safely ignore.
func (s *FileStore) CompactLifecycle(ctx context.Context, sessionID string, snapshot agent.PersistentSnapshot) error {
	if err := session.ValidateID(sessionID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before compacting agent lifecycle: %w", err)
	}
	return s.withSessionLock(ctx, sessionID, func() error {
		resources, err := session.ResolveResources(s.root, sessionID)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(resources.Root, 0o700); err != nil {
			return fmt.Errorf("create session directory: %w", err)
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("encode compacted agent snapshot: %w", err)
		}
		if len(data) > maxAgentSnapshotBytes {
			return fmt.Errorf("agent snapshot exceeds %d bytes", maxAgentSnapshotBytes)
		}
		if err := writeAgentSnapshot(ctx, resources.Root, resources.Agents, data); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("before truncating agent event journal: %w", err)
		}
		file, err := os.OpenFile(resources.AgentEvents, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("truncate agent event journal: %w", err)
		}
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			return fmt.Errorf("protect compacted agent event journal: %w", err)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return fmt.Errorf("sync compacted agent event journal: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close compacted agent event journal: %w", err)
		}
		return nil
	})
}
