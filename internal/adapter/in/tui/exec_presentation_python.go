package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	pytestCountRE   = regexp.MustCompile(`(?i)(\d+)\s+(passed|failed|skipped|xfailed|xpassed|errors?|warnings?)`)
	unittestRanRE   = regexp.MustCompile(`(?mi)^Ran\s+(\d+)\s+tests?\b`)
	unittestFailRE  = regexp.MustCompile(`(?mi)^FAILED\s*\(([^)]*)\)`)
	pythonVersionRE = regexp.MustCompile(`^python(?:\d+(?:\.\d+)*)?$`)
)

func isPythonExecutable(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "py" || pythonVersionRE.MatchString(name)
}

func pythonAction(args []string) string {
	if len(args) == 0 {
		return ""
	}
	switch args[0] {
	case "-c", "--command":
		return "eval"
	case "-m":
		if len(args) > 1 {
			module := args[1]
			switch module {
			case "pytest", "unittest", "compileall", "py_compile", "pip":
				return module
			default:
				return "module " + module
			}
		}
		return "module"
	case "--version", "-V":
		return "version"
	}
	if strings.HasPrefix(args[0], "-") {
		return ""
	}
	return args[0]
}

func pythonExecTitle(args []string, action string) string {
	switch action {
	case "eval":
		return "Python eval"
	case "pytest":
		return "Python pytest"
	case "unittest":
		return "Python unittest"
	case "compileall":
		return "Python compileall"
	case "py_compile":
		return "Python py_compile"
	case "pip":
		if len(args) > 2 && args[0] == "-m" {
			return execTitle("Pip", args[2])
		}
		return "Pip"
	case "version":
		return "Python version"
	}
	if action != "" {
		return execTitle("Python", action)
	}
	return "Python"
}

func summarizePythonExec(p *execPresentation, output string) {
	switch p.Action {
	case "pytest":
		summarizePytestExec(p, output)
	case "unittest":
		summarizeUnittestExec(p, output)
	case "compileall", "py_compile":
		if strings.TrimSpace(output) == "" {
			p.SuccessSummary = "valid syntax"
		}
	case "pip":
		lines := filterExecLines(output, func(line string) bool {
			lower := strings.ToLower(line)
			return strings.Contains(lower, "successfully installed") ||
				strings.Contains(lower, "successfully uninstalled") ||
				strings.Contains(lower, "requirement already satisfied")
		}, 4)
		p.Details = lines
		p.SuppressRaw = len(lines) > 0
	}
}

func summarizePytestExec(p *execPresentation, output string) {
	counts := map[string]int{}
	for _, match := range pytestCountRE.FindAllStringSubmatch(output, -1) {
		if len(match) != 3 {
			continue
		}
		n, err := strconv.Atoi(match[1])
		if err == nil {
			counts[strings.ToLower(match[2])] = n
		}
	}
	parts := make([]string, 0, 5)
	for _, key := range []string{"passed", "failed", "errors", "error", "skipped", "xfailed"} {
		if n := counts[key]; n > 0 {
			label := key
			if key == "error" || key == "errors" {
				label = pluralWord(n, "error", "errors")
			}
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	if len(parts) > 0 {
		p.Summary = strings.Join(parts, " · ")
		p.SuppressRaw = counts["failed"] == 0 && counts["error"] == 0 && counts["errors"] == 0
	}
}

func summarizeUnittestExec(p *execPresentation, output string) {
	match := unittestRanRE.FindStringSubmatch(output)
	if len(match) != 2 {
		return
	}
	total, err := strconv.Atoi(match[1])
	if err != nil {
		return
	}
	failed, errors := 0, 0
	if failure := unittestFailRE.FindStringSubmatch(output); len(failure) == 2 {
		for _, part := range strings.Split(failure[1], ",") {
			pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(pair) != 2 {
				continue
			}
			n, parseErr := strconv.Atoi(strings.TrimSpace(pair[1]))
			if parseErr != nil {
				continue
			}
			switch strings.TrimSpace(pair[0]) {
			case "failures":
				failed = n
			case "errors":
				errors = n
			}
		}
	}
	passed := maxInt(0, total-failed-errors)
	parts := []string{fmt.Sprintf("%d passed", passed)}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", errors, pluralWord(errors, "error", "errors")))
	}
	p.Summary = strings.Join(parts, " · ")
	p.SuppressRaw = failed == 0 && errors == 0
}

func pluralWord(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
