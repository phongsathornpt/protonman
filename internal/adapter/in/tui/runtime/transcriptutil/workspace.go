package transcriptutil

import (
	"os"
	"path/filepath"
	"strings"
)

// FormatWorkspaceDisplay formats a path for display, abbreviating the user home directory with ~.
func FormatWorkspaceDisplay(dir string) string {
	cleaned := strings.TrimSpace(dir)
	if cleaned == "" {
		return ""
	}
	cleaned = filepath.Clean(cleaned)
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if cleaned == home {
			return "~"
		}
		sep := string(filepath.Separator)
		if strings.HasPrefix(cleaned, home+sep) {
			return "~" + cleaned[len(home):]
		}
	}
	return cleaned
}

// DetectGitBranch attempts to locate the current git branch for a directory.
func DetectGitBranch(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	curr := filepath.Clean(dir)
	if abs, err := filepath.Abs(curr); err == nil {
		curr = abs
	}
	for i := 0; i < 16; i++ {
		gitPath := filepath.Join(curr, ".git")
		fi, err := os.Stat(gitPath)
		if err == nil {
			headFile := filepath.Join(gitPath, "HEAD")
			if !fi.IsDir() {
				data, rerr := os.ReadFile(gitPath)
				if rerr == nil {
					line := strings.TrimSpace(string(data))
					if strings.HasPrefix(line, "gitdir:") {
						target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
						if !filepath.IsAbs(target) {
							target = filepath.Join(curr, target)
						}
						headFile = filepath.Join(target, "HEAD")
					}
				}
			}
			headData, herr := os.ReadFile(headFile)
			if herr == nil {
				headStr := strings.TrimSpace(string(headData))
				if strings.HasPrefix(headStr, "ref: refs/heads/") {
					return strings.TrimPrefix(headStr, "ref: refs/heads/")
				}
				if len(headStr) >= 7 {
					return headStr[:7]
				}
			}
			return ""
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return ""
}
