package execview

import (
	"regexp"
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
			parsed := true
			for _, m := range matches {
				passed, okPassed := atoiExec(m[1])
				failed, okFailed := atoiExec(m[2])
				ignored, okIgnored := atoiExec(m[3])
				if !okPassed || !okFailed || !okIgnored {
					parsed = false
					break
				}
				counts.Passed += passed
				counts.Failed += failed
				counts.Ignored += ignored
			}
			if parsed {
				p.Summary = formatTestCounts(counts)
				p.SuppressRaw = counts.Failed == 0
				if counts.Failed > 0 {
					p.Details = firstFailureLines(output, 3)
				}
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
