package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m bubbleModel) welcomeCard() string {
	mode := layoutModeForHeight(m.height)
	if mode != layoutNormal || m.width < 60 {
		return brandLockup(m.width)
	}

	rows := []string{
		brandLockup(m.width),
		"",
	}

	if ws := formatWorkspaceDisplay(m.workDir); ws != "" {
		branch := detectGitBranch(m.workDir)
		branchBadge := ""
		if branch != "" {
			branchBadge = " " + mutedStyle.Render("git:(") + systemStyle.Render(branch) + mutedStyle.Render(")")
		}
		rows = append(rows, heroLabelStyle.Render("Workspace ")+bodyStyle.Render(ws)+branchBadge, "")
	}

	if m.width >= 80 {
		colWidth := 36
		rows = append(rows,
			heroLabelStyle.Render("Quick Actions"),
			"  "+padToWidth(heroKeyStyle.Render("› /help")+"   "+mutedStyle.Render("Command palette"), colWidth)+heroKeyStyle.Render("› Ctrl+P")+"  "+mutedStyle.Render("Switch model"),
			"  "+padToWidth(heroKeyStyle.Render("› /model")+"  "+mutedStyle.Render("Choose AI provider"), colWidth)+heroKeyStyle.Render("› Ctrl+T")+"  "+mutedStyle.Render("View transcript"),
			"  "+padToWidth(heroKeyStyle.Render("› /skills")+" "+mutedStyle.Render("Active capabilities"), colWidth)+heroKeyStyle.Render("› ! <cmd>")+" "+mutedStyle.Render("Run bash command"),
		)
	} else {
		rows = append(rows,
			heroLabelStyle.Render("Quick Actions"),
			"  "+heroKeyStyle.Render("› /help")+"     "+mutedStyle.Render("Command palette & shortcuts"),
			"  "+heroKeyStyle.Render("› /model")+"    "+mutedStyle.Render("Switch AI model (Ctrl+P)"),
			"  "+heroKeyStyle.Render("› /skills")+"   "+mutedStyle.Render("Inspect loaded capabilities"),
			"  "+heroKeyStyle.Render("› ! <cmd>")+"   "+mutedStyle.Render("Run bash command directly"),
		)
	}

	rows = append(rows,
		"",
		mutedStyle.Render("Tip: Type a prompt to inspect or change this project, or ask for guidance."),
	)

	return strings.Join(rows, "\n")
}

func padToWidth(s string, targetWidth int) string {
	w := ansi.StringWidth(s)
	if w >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-w)
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

func promptPlaceholder(hasRunner bool) string {
	if hasRunner {
		return "Ask Protonman to inspect or change this workspace…"
	}
	return "Type a message or /command…"
}

func (m *bubbleModel) resetTranscript() {
	m.ensureHistoryState().Reset()
	m.syncLegacyBlocks()
	m.followTail = true
	m.showWelcome = true
	m.refreshTranscriptViewport(true)
}

// resetConversation clears both the visible transcript and provider history.
// Keeping this separate from resetTranscript makes Ctrl+L a safe display-only
// action while /new has the explicit semantics users expect from its name.
func (m *bubbleModel) resetConversation() {
	m.ensureHistoryState().Reset()
	m.messages = nil
	m.queue = nil
	m.followTail = true
	m.showWelcome = true
	if m.skills != nil {
		m.skills.ResetActivated()
	}
	m.refreshTranscriptViewport(true)
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
