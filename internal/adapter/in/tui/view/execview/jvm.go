package execview

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

func summarizeJVMExec(p *Presentation, output string) {
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
	attachFailureDetails(p, output)
}

func summarizeMavenExec(p *Presentation, output string) {
	matches := mavenTestsRE.FindAllStringSubmatch(output, -1)
	if len(matches) > 0 {
		counts := testCounts{}
		parsed := true
		for _, m := range matches {
			total, okTotal := atoiExec(m[1])
			failed, okFailed := atoiExec(m[2])
			errs, okErrs := atoiExec(m[3])
			skipped, okSkipped := atoiExec(m[4])
			if !okTotal || !okFailed || !okErrs || !okSkipped {
				parsed = false
				break
			}
			counts.Passed += total - failed - errs - skipped
			counts.Failed += failed
			counts.Errors += errs
			counts.Skipped += skipped
		}
		if parsed {
			p.Summary = formatTestCounts(counts)
			p.SuppressRaw = counts.Failed == 0 && counts.Errors == 0
		}
	}
	attachFailureDetails(p, output)
}

func summarizeGradleExec(p *Presentation, output string) {
	if m := gradleTestsRE.FindStringSubmatch(output); len(m) > 0 {
		total, okTotal := atoiExec(m[1])
		failed := 0
		okFailed := true
		if len(m) > 2 && m[2] != "" {
			failed, okFailed = atoiExec(m[2])
		}
		if okTotal && okFailed {
			p.Summary = formatTestCounts(testCounts{Passed: total - failed, Failed: failed})
			p.SuppressRaw = failed == 0
		}
	} else if strings.Contains(output, "BUILD SUCCESSFUL") {
		p.SuccessSummary, p.SuppressRaw = "build successful", true
	}
	attachFailureDetails(p, output)
}
