package execview

import (
	"regexp"
	"strings"
)

var (
	dotnetTestRE  = regexp.MustCompile(`Failed:\s*(\d+),\s*Passed:\s*(\d+),\s*Skipped:\s*(\d+)`)
	dotnetBuildRE = regexp.MustCompile(`(?m)^\s*(\d+) Warning\(s\)\s*$[\s\S]*?^\s*(\d+) Error\(s\)\s*$`)
)

func dotnetExecTitle(_ string, _ []string, action string) string { return execTitle("Dotnet", action) }

func summarizeDotnetExec(p *Presentation, output string) {
	switch p.Action {
	case "test":
		if m := dotnetTestRE.FindStringSubmatch(output); len(m) == 4 {
			failed, passed, skipped := atoiExec(m[1]), atoiExec(m[2]), atoiExec(m[3])
			p.Summary = formatTestCounts(testCounts{Passed: passed, Failed: failed, Skipped: skipped})
			p.SuppressRaw = failed == 0
		}
	case "build":
		if m := dotnetBuildRE.FindStringSubmatch(output); len(m) == 3 {
			warnings, errs := atoiExec(m[1]), atoiExec(m[2])
			p.Summary = formatDiagnosticCounts(diagnosticCounts{Errors: errs, Warnings: warnings})
			if p.Summary == "" {
				p.SuccessSummary = "build succeeded"
			}
			p.SuppressRaw = errs == 0
		} else if strings.Contains(output, "Build succeeded.") {
			p.SuccessSummary, p.SuppressRaw = "build succeeded", true
		}
	}
	if failures := firstFailureLines(output, 3); len(failures) > 0 {
		p.Details = failures
	}
}
