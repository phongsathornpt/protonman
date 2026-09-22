package execview

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	phpunitOKRE    = regexp.MustCompile(`OK \((\d+) tests?`)
	phpunitTestsRE = regexp.MustCompile(`Tests:\s*(\d+)(?:,[^\n]*?Failures:\s*(\d+))?(?:,[^\n]*?Errors:\s*(\d+))?(?:,[^\n]*?Skipped:\s*(\d+))?`)
)

func isPHPExecutable(name string) bool {
	base := filepath.Base(name)
	return base == "php" || base == "phpunit" || base == "composer"
}

func phpAction(args []string) string {
	for i, arg := range args {
		switch arg {
		case "-r":
			return "eval"
		case "-l":
			return "lint"
		case "-d":
			if i+1 < len(args) {
				i++
			}
		}
	}
	return firstNonFlagArg(args)
}

func phpExecTitle(name string, args []string, action string) string {
	base := filepath.Base(name)
	switch base {
	case "phpunit":
		return "PHPUnit"
	case "composer":
		return execTitle("Composer", action)
	}
	if action == "lint" {
		for i, arg := range args {
			if arg == "-l" && i+1 < len(args) {
				return execTitle("PHP lint", args[i+1])
			}
		}
		return "PHP lint"
	}
	if action == "eval" {
		return "PHP eval"
	}
	return execTitle("PHP", action)
}

func summarizePHPExec(p *Presentation, output string) {
	if p.Title == "PHPUnit" {
		summarizePHPUnitExec(p, output)
		return
	}
	if p.Action == "lint" && strings.Contains(strings.ToLower(output), "no syntax errors detected") {
		p.Summary, p.SuppressRaw = "valid syntax", true
	}
	attachFailureDetails(p, output)
}

func summarizePHPUnitExec(p *Presentation, output string) {
	if m := phpunitTestsRE.FindStringSubmatch(output); len(m) > 0 {
		total, okTotal := atoiExec(m[1])
		failed, errs, skipped := 0, 0, 0
		okFailed, okErrs, okSkipped := true, true, true
		// A non-participating optional capture means 0; only a failed parse
		// of a present capture aborts the summary (fail-safe).
		if len(m) > 2 && m[2] != "" {
			failed, okFailed = atoiExec(m[2])
		}
		if len(m) > 3 && m[3] != "" {
			errs, okErrs = atoiExec(m[3])
		}
		if len(m) > 4 && m[4] != "" {
			skipped, okSkipped = atoiExec(m[4])
		}
		if okTotal && okFailed && okErrs && okSkipped {
			p.Summary = formatTestCounts(testCounts{Passed: total - failed - errs - skipped, Failed: failed, Errors: errs, Skipped: skipped})
			p.SuppressRaw = failed == 0 && errs == 0
		}
	} else if m := phpunitOKRE.FindStringSubmatch(output); len(m) == 2 {
		passed, ok := atoiExec(m[1])
		if ok {
			p.Summary, p.SuppressRaw = formatTestCounts(testCounts{Passed: passed}), true
		}
	}
	attachFailureDetails(p, output)
}
