package tui

import (
	"regexp"
	"strings"
)

var (
	mavenTestsRE  = regexp.MustCompile(`Tests run:\s*(\d+),\s*Failures:\s*(\d+),\s*Errors:\s*(\d+),\s*Skipped:\s*(\d+)`)
	gradleTestsRE = regexp.MustCompile(`(?i)(\d+) tests completed(?:,\s*(\d+) failed)?`)
)

func jvmAction(args []string) string { return firstNonFlagArg(args) }

func jvmExecTitle(name string, _ []string, action string) string {
	switch name {
	case "gradle", "gradlew":
		return execTitle("Gradle", action)
	case "mvn", "mvnw":
		return execTitle("Maven", action)
	case "javac":
		return execTitle("Javac", action)
	default:
		return execTitle("Java", action)
	}
}

func summarizeJVMExec(p *execPresentation, output string) {
	if strings.HasPrefix(p.Title, "Maven ") {
		summarizeMavenExec(p, output)
		return
	}
	if strings.HasPrefix(p.Title, "Gradle ") {
		summarizeGradleExec(p, output)
		return
	}
	if strings.HasPrefix(p.Title, "Javac ") && strings.TrimSpace(output) == "" {
		p.SuppressRaw = true
	}
	if failures := firstFailureLines(output, 3); len(failures) > 0 {
		p.Details = failures
	}
}

func summarizeMavenExec(p *execPresentation, output string) {
	matches := mavenTestsRE.FindAllStringSubmatch(output, -1)
	if len(matches) > 0 {
		counts := testCounts{}
		for _, m := range matches {
			total, failed, errs, skipped := atoiExec(m[1]), atoiExec(m[2]), atoiExec(m[3]), atoiExec(m[4])
			counts.Passed += total - failed - errs - skipped
			counts.Failed += failed
			counts.Errors += errs
			counts.Skipped += skipped
		}
		p.Summary = formatTestCounts(counts)
		p.SuppressRaw = counts.Failed == 0 && counts.Errors == 0
	}
	if failures := firstFailureLines(output, 3); len(failures) > 0 {
		p.Details = failures
	}
}

func summarizeGradleExec(p *execPresentation, output string) {
	if m := gradleTestsRE.FindStringSubmatch(output); len(m) > 0 {
		total, failed := atoiExec(m[1]), 0
		if len(m) > 2 && m[2] != "" {
			failed = atoiExec(m[2])
		}
		p.Summary = formatTestCounts(testCounts{Passed: total - failed, Failed: failed})
		p.SuppressRaw = failed == 0
	} else if strings.Contains(output, "BUILD SUCCESSFUL") {
		p.SuccessSummary, p.SuppressRaw = "build successful", true
	}
	if failures := firstFailureLines(output, 3); len(failures) > 0 {
		p.Details = failures
	}
}
