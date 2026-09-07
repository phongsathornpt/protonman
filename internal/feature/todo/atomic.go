package todo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func writeAtomic(ctx context.Context, path string, content []byte) (writeErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create todo directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("protect todo directory: %w", err)
	}
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat todo file: %w", statErr)
	}
	file, err := os.CreateTemp(dir, ".todo-*.tmp")
	if err != nil {
		return fmt.Errorf("create todo temp file: %w", err)
	}
	tmp := file.Name()
	defer func() { _ = file.Close(); _ = os.Remove(tmp) }()
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("chmod todo temp file: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("write todo temp file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync todo temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close todo temp file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("install todo markdown: %w", err)
	}
	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open todo directory for sync: %w", err)
	}
	defer dirFile.Close()
	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("sync todo directory: %w", err)
	}
	return nil
}

func withFileLock(ctx context.Context, path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create todo lock directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("protect todo lock directory: %w", err)
	}
	unlock, err := lockFileContext(ctx, path+".lock")
	if err != nil {
		return fmt.Errorf("acquire todo lock: %w", err)
	}
	defer unlock()
	return fn()
}
