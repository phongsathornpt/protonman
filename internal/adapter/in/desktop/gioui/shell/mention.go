//go:build desktop || desktop_gio

package shell

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
	"github.com/phongsathornpt/protonman/internal/base/glob"
)

// MentionItem and MentionItemKind live in component/conversation alongside the
// suggestion list they render. The workspace index below produces them, so the
// aliases keep a single definition of the suggestion shape.
type MentionItem = conversationcomponent.MentionItem

type MentionItemKind = conversationcomponent.MentionItemKind

const (
	MentionItemKindAgent = conversationcomponent.MentionItemKindAgent
	MentionItemKindFile  = conversationcomponent.MentionItemKindFile
	MentionItemKindDir   = conversationcomponent.MentionItemKindDir
)

const (
	maxWorkspaceScanItems = 3000
	maxWorkspaceScanDepth = 8
	workspaceCacheTTL     = 15 * time.Second
)

type workspaceMentionCache struct {
	mu          sync.Mutex
	workDir     string
	items       []MentionItem
	lastIndexed time.Time
	refreshing  bool
}

func (c *workspaceMentionCache) get(workDir string) []MentionItem {
	if strings.TrimSpace(workDir) == "" {
		return nil
	}
	c.mu.Lock()
	if c.workDir == workDir && len(c.items) > 0 {
		items := c.items
		stale := time.Since(c.lastIndexed) >= workspaceCacheTTL
		shouldRefresh := stale && !c.refreshing
		if shouldRefresh {
			c.refreshing = true
		}
		c.mu.Unlock()
		if shouldRefresh {
			go c.refresh(workDir)
		}
		return items
	}
	if c.workDir == workDir && c.refreshing {
		c.mu.Unlock()
		return nil
	}
	c.refreshing = true
	c.mu.Unlock()
	go c.refresh(workDir)
	return nil
}

func (c *workspaceMentionCache) refresh(workDir string) {
	items := scanWorkspaceFiles(workDir)
	c.mu.Lock()
	c.workDir = workDir
	c.items = items
	c.lastIndexed = time.Now()
	c.refreshing = false
	c.mu.Unlock()
}

func scanWorkspaceFiles(workDir string) []MentionItem {
	if strings.TrimSpace(workDir) == "" {
		return nil
	}

	ignorePatterns := loadGitIgnore(workDir)
	var items []MentionItem

	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxWorkspaceScanDepth || len(items) >= maxWorkspaceScanItems {
			return
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}

		for _, entry := range entries {
			name := entry.Name()
			if isIgnoredDefault(name) {
				continue
			}

			fullPath := filepath.Join(dir, name)
			relPath, err := filepath.Rel(workDir, fullPath)
			if err != nil {
				continue
			}

			if matchesAnyPattern(relPath, name, ignorePatterns) {
				continue
			}

			if entry.IsDir() {
				items = append(items, MentionItem{
					Kind:        MentionItemKindDir,
					Name:        relPath,
					Description: "Directory",
					PrefixTag:   "[dir]",
				})
				walk(fullPath, depth+1)
			} else {
				info, err := entry.Info()
				sizeStr := ""
				var sz int64
				if err == nil {
					sz = info.Size()
					if sz >= 1024*1024 {
						sizeStr = fmt.Sprintf("%.1f MB", float64(sz)/(1024*1024))
					} else if sz >= 1024 {
						sizeStr = fmt.Sprintf("%d KB", sz/1024)
					} else {
						sizeStr = fmt.Sprintf("%d B", sz)
					}
				}
				items = append(items, MentionItem{
					Kind:        MentionItemKindFile,
					Name:        relPath,
					Description: sizeStr,
					PrefixTag:   "[file]",
					SizeBytes:   sz,
				})
			}

			if len(items) >= maxWorkspaceScanItems {
				return
			}
		}
	}

	walk(workDir, 0)
	return items
}

func isIgnoredDefault(name string) bool {
	switch name {
	case ".git", "node_modules", "target", "bin", "dist", ".idea", ".vscode", ".bak":
		return true
	}
	return strings.HasPrefix(name, ".") && len(name) > 1
}

func loadGitIgnore(workDir string) []string {
	path := filepath.Join(workDir, ".gitignore")
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func matchesAnyPattern(relPath, baseName string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimPrefix(p, "/")
		p = strings.TrimSuffix(p, "/")
		if glob.Match(p, relPath) || glob.Match(p, baseName) {
			return true
		}
	}
	return false
}
