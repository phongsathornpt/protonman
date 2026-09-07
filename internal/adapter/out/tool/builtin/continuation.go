package builtin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
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
