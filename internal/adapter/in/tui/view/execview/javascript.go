package execview

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	bunPassRE     = regexp.MustCompile(`(?mi)^\s*(\d+)\s+pass(?:ed)?\b`)
	bunFailRE     = regexp.MustCompile(`(?mi)^\s*(\d+)\s+fail(?:ed)?\b`)
	nodePassRE    = regexp.MustCompile(`(?mi)^#\s*pass\s+(\d+)\s*$`)
	nodeFailRE    = regexp.MustCompile(`(?mi)^#\s*fail\s+(\d+)\s*$`)
	viteModulesRE = regexp.MustCompile(`(?i)✓?\s*(\d+)\s+modules?\s+transformed`)
)

func packageRunnerAction(args []string) string {
	if len(args) == 0 {
		return ""
	}
	if args[0] == "run" && len(args) > 1 {
		return "run " + args[1]
	}
	return args[0]
}

func nodeAction(args []string) string {
	for _, arg := range args {
		switch {
		case arg == "--test" || strings.HasPrefix(arg, "--test="):
			return "test"
		case arg == "--check" || arg == "-c":
			return "check"
		case arg == "-e" || arg == "--eval":
			return "eval"
		case arg == "-p" || arg == "--print":
			return "print"
		}
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0]
	}
	return ""
}

func nodeExecTitle(args []string, action string) string {
	switch action {
	case "test":
		return "Node test"
	case "eval":
		return "Node eval"
	case "print":
		return "Node print"
	case "check":
		for i, arg := range args {
			if (arg == "--check" || arg == "-c") && i+1 < len(args) {
				return execTitle("Node check", args[i+1])
			}
		}
		return "Node check"
	}
	if action != "" {
		return execTitle("Node", action)
	}
	return "Node"
}

func frameworkAction(action, command, framework string) string {
	action = strings.TrimSpace(action)
	if action == "run dev" || action == "dev" || action == "" && strings.Contains(command, framework) {
		return "dev"
	}
	if strings.Contains(action, "build") || strings.Contains(command, framework+" build") {
		return "build"
	}
	if strings.Contains(action, "start") || strings.Contains(command, framework+" start") {
		return "start"
	}
	if strings.HasPrefix(action, "run ") {
		return strings.TrimPrefix(action, "run ")
	}
	return action
}

func summarizeBunExec(p *Presentation, output string) {
	switch {
	case p.Action == "test":
		passed := firstRegexpInt(bunPassRE, output)
		failed := firstRegexpInt(bunFailRE, output)
		parts := []string{}
		if passed >= 0 {
			parts = append(parts, fmt.Sprintf("%d passed", passed))
		}
		if failed > 0 {
			parts = append(parts, fmt.Sprintf("%d failed", failed))
		}
		if len(parts) > 0 {
			p.Summary = strings.Join(parts, " · ")
			p.SuppressRaw = failed == 0
		}
	case p.Action == "install" || p.Action == "add" || p.Action == "remove":
		lines := filterExecLines(output, func(line string) bool {
			lower := strings.ToLower(line)
			return strings.Contains(lower, "installed") || strings.Contains(lower, "saved lockfile") || strings.HasPrefix(line, "+ ") || strings.HasPrefix(line, "- ")
		}, 5)
		p.Details = lines
		p.SuppressRaw = len(lines) > 0
	case p.Action == "build":
		p.Details = capExecLines(nonEmptyExecLines(output), 5)
		p.SuppressRaw = len(p.Details) > 0
	}
}

func summarizeNodeExec(p *Presentation, output string) {
	switch p.Action {
	case "test":
		passed := firstRegexpInt(nodePassRE, output)
		failed := firstRegexpInt(nodeFailRE, output)
		if passed >= 0 || failed >= 0 {
			parts := []string{}
			if passed >= 0 {
				parts = append(parts, fmt.Sprintf("%d passed", passed))
			}
			if failed > 0 {
				parts = append(parts, fmt.Sprintf("%d failed", failed))
			}
			p.Summary = strings.Join(parts, " · ")
			p.SuppressRaw = failed <= 0
		}
	case "check":
		if strings.TrimSpace(output) == "" {
			p.SuccessSummary = "valid syntax"
		}
	}
}

func summarizeViteExec(p *Presentation, output string) {
	switch p.Action {
	case "build":
		if matches := viteModulesRE.FindStringSubmatch(output); len(matches) == 2 {
			p.Summary = matches[1] + " modules"
		}
		p.Details = filterExecLines(output, func(line string) bool {
			return strings.Contains(line, "dist/") || strings.Contains(strings.ToLower(line), "warning")
		}, 5)
		p.SuppressRaw = p.Summary != "" || len(p.Details) > 0
	default:
		urls := filterExecLines(output, func(line string) bool {
			lower := strings.ToLower(line)
			return strings.Contains(lower, "local:") || strings.Contains(lower, "network:") || strings.Contains(lower, "http://") || strings.Contains(lower, "https://")
		}, 4)
		if len(urls) > 0 {
			p.Summary = "ready"
			p.Details = urls
			p.SuppressRaw = true
		}
	}
}

func summarizeNextExec(p *Presentation, output string) {
	switch p.Action {
	case "build":
		routes := filterExecLines(output, func(line string) bool {
			trimmed := strings.TrimSpace(line)
			return strings.Contains(trimmed, " /") && (strings.HasPrefix(trimmed, "○") || strings.HasPrefix(trimmed, "ƒ") || strings.HasPrefix(trimmed, "●") || strings.HasPrefix(trimmed, "λ") || strings.HasPrefix(trimmed, "┌") || strings.HasPrefix(trimmed, "├") || strings.HasPrefix(trimmed, "└"))
		}, 5)
		if len(routes) > 0 {
			count := 0
			for _, line := range nonEmptyExecLines(output) {
				trimmed := strings.TrimSpace(line)
				if strings.Contains(trimmed, " /") && (strings.HasPrefix(trimmed, "○") || strings.HasPrefix(trimmed, "ƒ") || strings.HasPrefix(trimmed, "●") || strings.HasPrefix(trimmed, "λ") || strings.HasPrefix(trimmed, "┌") || strings.HasPrefix(trimmed, "├") || strings.HasPrefix(trimmed, "└")) {
					count++
				}
			}
			p.Summary = pluralCount(count, "route", "routes")
			p.Details = routes
			p.SuppressRaw = true
		}
	default:
		urls := filterExecLines(output, func(line string) bool {
			lower := strings.ToLower(line)
			return strings.Contains(lower, "local:") || strings.Contains(lower, "network:") || strings.Contains(lower, "http://") || strings.Contains(lower, "https://")
		}, 4)
		if len(urls) > 0 {
			p.Summary = "ready"
			p.Details = urls
			p.SuppressRaw = true
		}
	}
}

func firstRegexpInt(re *regexp.Regexp, value string) int {
	match := re.FindStringSubmatch(value)
	if len(match) != 2 {
		return -1
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return n
}
