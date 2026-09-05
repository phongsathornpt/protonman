package builtin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/projectTHORN/proton/internal/workspace"
)

func continuationToken(toolName string, query any, snapshot string) (string, error) {
	canonical, err := json.Marshal(query)
	if err != nil {
		return "", fmt.Errorf("encode continuation query: %w", err)
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(toolName))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(canonical)
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(snapshot))
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fileSnapshot(info os.FileInfo) string {
	return fmt.Sprintf("%d:%d:%d", info.Size(), info.ModTime().UnixNano(), uint32(info.Mode()))
}

func directorySnapshot(ctx context.Context, root string, entries []os.DirEntry) (string, error) {
	hash := sha256.New()
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		info, err := entry.Info()
		if err != nil {
			return "", fmt.Errorf("stat directory entry %q: %w", entry.Name(), err)
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%d\x00%d\n", entry.Name(), info.Size(), info.ModTime().UnixNano(), uint32(info.Mode()))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func treeSnapshot(ctx context.Context, workspaceRoot *workspace.Workspace, root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path != root && workspaceRoot.IsProtected(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" && path != root {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%d\x00%d\n", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano(), uint32(info.Mode()))
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
