package workspace

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/projectTHORN/proton/internal/tool"
)

var ErrPreexistingWorkspaceChange = errors.New("pre-existing workspace change")

type mutationSessionKey struct{}

type MutationSession struct {
	mu    sync.Mutex
	owned map[string]map[string]struct{}
}

func WithMutationSession(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := MutationSessionFromContext(ctx); ok {
		return ctx
	}
	return context.WithValue(ctx, mutationSessionKey{}, &MutationSession{owned: make(map[string]map[string]struct{})})
}
func MutationSessionFromContext(ctx context.Context) (*MutationSession, bool) {
	if ctx == nil {
		return nil, false
	}
	session, ok := ctx.Value(mutationSessionKey{}).(*MutationSession)
	return session, ok && session != nil
}

func (w *Workspace) GuardWholeFileMutation(ctx context.Context, paths ...string) error {
	session, ok := MutationSessionFromContext(ctx)
	if !ok || w == nil || len(paths) == 0 {
		return nil
	}
	dirty, err := w.gitDirtyPaths(ctx)
	if err != nil || len(dirty) == 0 {
		return nil
	}
	root := filepath.Clean(w.root)
	session.mu.Lock()
	defer session.mu.Unlock()
	owned := session.owned[root]
	for _, path := range paths {
		rel, relErr := filepath.Rel(root, filepath.Clean(path))
		if relErr != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			continue
		}
		rel = filepath.Clean(rel)
		if _, isDirty := dirty[rel]; !isDirty {
			continue
		}
		if _, isOwned := owned[rel]; isOwned {
			continue
		}
		return tool.WrapToolError(tool.ErrorCodePreexistingWorkspaceChange,
			fmt.Sprintf("refusing to overwrite pre-existing workspace changes in %q", rel), ErrPreexistingWorkspaceChange)
	}
	return nil
}
func (w *Workspace) MarkMutationOwned(ctx context.Context, paths ...string) {
	session, ok := MutationSessionFromContext(ctx)
	if !ok || w == nil || len(paths) == 0 {
		return
	}
	root := filepath.Clean(w.root)
	session.mu.Lock()
	defer session.mu.Unlock()
	owned := session.owned[root]
	if owned == nil {
		owned = make(map[string]struct{})
		session.owned[root] = owned
	}
	for _, path := range paths {
		rel, err := filepath.Rel(root, filepath.Clean(path))
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		owned[filepath.Clean(rel)] = struct{}{}
	}
}

func (w *Workspace) gitDirtyPaths(ctx context.Context) (map[string]struct{}, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", w.root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, err
	}
	return parseGitPorcelainZ(output), nil
}
func parseGitPorcelainZ(output []byte) map[string]struct{} {
	parts := strings.Split(string(output), "\x00")
	dirty := make(map[string]struct{}, len(parts))
	for i := 0; i < len(parts); i++ {
		record := parts[i]
		if len(record) < 4 {
			continue
		}
		status := record[:2]
		path := filepath.Clean(record[3:])
		if path != "." && path != "" {
			dirty[path] = struct{}{}
		}
		if strings.ContainsAny(status, "RC") && i+1 < len(parts) && parts[i+1] != "" {
			i++
			original := filepath.Clean(parts[i])
			if original != "." && original != "" {
				dirty[original] = struct{}{}
			}
		}
	}
	return dirty
}
