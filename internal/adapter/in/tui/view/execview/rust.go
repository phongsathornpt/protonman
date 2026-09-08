package execview

import (
	"regexp"
	"strconv"
	"strings"
)

var cargoTestResultRE = regexp.MustCompile(`test result: (?:ok|FAILED)\.\s+(\d+) passed;\s+(\d+) failed;\s+(\d+) ignored`)

func rustAction(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func rustExecTitle(name string, args []string, action string) string {
	switch name {
	case "cargo":
		return execTitle("Cargo", action)
	case "rustfmt":
		return execTitle("Rustfmt", firstNonFlagArg(args))
	default:
		return execTitle("Rustc", firstNonFlagArg(args))
	}
}

func summarizeRustExec(p *Presentation, output string) {
	if p.Title == "" {
		return
	}
	switch p.Action {
	case "test":
		matches := cargoTestResultRE.FindAllStringSubmatch(output, -1)
		if len(matches) > 0 {
			counts := testCounts{}
			for _, m := range matches {
				counts.Passed += atoiExec(m[1])
				counts.Failed += atoiExec(m[2])
				counts.Ignored += atoiExec(m[3])
			}
			p.Summary = formatTestCounts(counts)
			p.SuppressRaw = counts.Failed == 0
			if counts.Failed > 0 {
				p.Details = firstFailureLines(output, 3)
			}
		}
	case "check", "build", "clippy":
		counts := diagnosticCounts{
			Errors:   countExecLines(output, "error"),
			Warnings: countExecLines(output, "warning"),
		}
		if summary := formatDiagnosticCounts(counts); summary != "" {
			p.Summary = summary
		}
		if counts.Errors > 0 {
			p.Details = firstFailureLines(output, 3)
		}
	case "fmt":
		if strings.TrimSpace(output) == "" {
			p.SuccessSummary, p.SuppressRaw = "clean", true
		}
	}
}

func atoiExec(value string) int { n, _ := strconv.Atoi(value); return n }

func countExecLines(output, prefix string) int {
	count := 0
	for _, line := range nonEmptyExecLines(output) {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, prefix+":") || strings.HasPrefix(lower, prefix+"[") {
			count++
		}
	}
	return count
}
