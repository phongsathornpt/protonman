package execview

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	gitDiffStatRE = regexp.MustCompile(`(?i)(\d+) files? changed(?:, (\d+) insertions?\(\+\))?(?:, (\d+) deletions?\(-\))?`)
	gitOnelineRE  = regexp.MustCompile(`^[0-9a-fA-F]{7,64}\s+`)
)

func gitAction(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-C" || arg == "-c" || arg == "--git-dir" || arg == "--work-tree" {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return ""
}

func summarizeGitExec(p *Presentation, output string) {
	lines := nonEmptyExecLines(output)
	switch p.Action {
	case "status":
		plainStatus := strings.HasPrefix(strings.TrimSpace(output), "On branch ") || strings.Contains(output, "Changes not staged for commit:") || strings.Contains(output, "Changes to be committed:")
		if strings.Contains(output, "nothing to commit, working tree clean") {
			p.Summary = "clean"
			p.SuppressRaw = true
			return
		}
		if plainStatus {
			if len(lines) > 0 {
				p.Summary = "working tree changes"
				p.Details = capExecLines(lines, 5)
				p.SuppressRaw = true
			}
			return
		}
		changed, untracked := 0, 0
		for _, line := range lines {
			if strings.HasPrefix(line, "??") {
				untracked++
			} else if len(line) >= 2 {
				changed++
			}
		}
		if changed+untracked == 0 {
			p.Summary = "clean"
		} else {
			parts := []string{}
			if changed > 0 {
				parts = append(parts, pluralCount(changed, "changed file", "changed files"))
			}
			if untracked > 0 {
				parts = append(parts, pluralCount(untracked, "untracked", "untracked"))
			}
			p.Summary = strings.Join(parts, " · ")
			p.Details = capExecLines(lines, 5)
		}
		p.SuppressRaw = true
	case "diff":
		if match := gitDiffStatRE.FindStringSubmatch(output); len(match) == 4 {
			files, _ := strconv.Atoi(match[1])
			adds, _ := strconv.Atoi(match[2])
			dels, _ := strconv.Atoi(match[3])
			parts := []string{pluralCount(files, "file", "files")}
			if adds > 0 || dels > 0 {
				parts = append(parts, fmt.Sprintf("+%d -%d", adds, dels))
			}
			p.Summary = strings.Join(parts, " · ")
			p.Details = capExecLines(lines, 5)
			p.SuppressRaw = true
			return
		}
		files, adds, dels := 0, 0, 0
		for _, line := range lines {
			switch {
			case strings.HasPrefix(line, "diff --git "):
				files++
			case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
				adds++
			case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
				dels++
			}
		}
		if files == 0 && len(lines) == 0 {
			p.Summary = "no changes"
		} else {
			parts := []string{}
			if files > 0 {
				parts = append(parts, pluralCount(files, "file", "files"))
			} else if strings.Contains(p.Title, "--name-only") {
				parts = append(parts, pluralCount(len(lines), "file", "files"))
			}
			if adds > 0 || dels > 0 {
				parts = append(parts, fmt.Sprintf("+%d -%d", adds, dels))
			}
			p.Summary = strings.Join(parts, " · ")
			p.Details = capExecLines(lines, 5)
		}
		p.SuppressRaw = true
	case "log":
		commits := 0
		for _, line := range lines {
			if strings.HasPrefix(line, "commit ") || gitOnelineRE.MatchString(strings.TrimSpace(line)) {
				commits++
			}
		}
		if commits == 0 && strings.Contains(p.Title, "--oneline") {
			commits = len(lines)
		}
		if commits > 0 {
			p.Summary = pluralCount(commits, "commit", "commits")
		}
		p.Details = capExecLines(lines, 5)
		p.SuppressRaw = len(lines) > 0
	case "show":
		p.Details = capExecLines(lines, 5)
		p.SuppressRaw = len(lines) > 0
	case "commit":
		if len(lines) > 0 {
			p.Summary = "committed"
			p.Details = capExecLines(lines, 2)
			p.SuppressRaw = true
		}
	case "push", "pull", "fetch", "merge", "rebase":
		if len(lines) > 0 {
			p.Details = tailExecLines(lines, 3)
			p.SuppressRaw = true
		}
	}
}
