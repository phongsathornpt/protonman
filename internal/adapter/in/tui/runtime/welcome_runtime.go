package runtime

import (
	"os"
	"path/filepath"
	"strings"
)

type welcomeCardCache struct {
	workDir     string
	branch      string
	branchValid bool
	width       int
	height      int
	rendered    string
	renderValid bool
}

func (m *bubbleModel) welcomeCard() string {
	if m == nil {
		return ""
	}
	cache := &m.welcomeCache
	if cache.workDir != m.workDir {
		*cache = welcomeCardCache{workDir: m.workDir}
	}
	if !cache.branchValid {
		cache.branch = detectGitBranch(m.workDir)
		cache.branchValid = true
		cache.renderValid = false
	}
	if cache.renderValid && cache.width == m.width && cache.height == m.height {
		return cache.rendered
	}
	cache.rendered = m.renderWelcomeCard(cache.branch)
	cache.width = m.width
	cache.height = m.height
	cache.renderValid = true
	return cache.rendered
}

func (m *bubbleModel) invalidateWelcomeBranch() {
	if m == nil {
		return
	}
	m.welcomeCache.branchValid = false
	m.welcomeCache.renderValid = false
}

func (m *bubbleModel) renderWelcomeCard(branch string) string {
	rows := []string{brandStyle.Render(glyphBrand + " protonman")}
	if ws := formatWorkspaceDisplay(m.workDir); ws != "" {
		workspace := ws
		if branch != "" {
			workspace += " · " + branch
		}
		rows = append(rows, mutedStyle.Render(truncateWithEllipsis(workspace, maxInt(1, m.width-2))))
	}
	return strings.Join(rows, "\n")
}

func formatWorkspaceDisplay(dir string) string {
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

func detectGitBranch(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	curr := filepath.Clean(dir)
	for i := 0; i < 4; i++ {
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
