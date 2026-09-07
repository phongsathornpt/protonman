package tui

import (
	"path/filepath"
	"regexp"
	"strings"
)

var rspecSummaryRE = regexp.MustCompile(`(?m)(\d+) examples?,\s*(\d+) failures?(?:,\s*(\d+) pending)?`)

func isRubyExecutable(name string) bool {
	base := filepath.Base(name)
	return base == "ruby" || base == "rspec" || base == "bundle"
}

func rubyAction(args []string) string {
	for i, arg := range args {
		switch arg {
		case "-e":
			return "eval"
		case "-c":
			return "check"
		case "exec":
			if i+1 < len(args) {
				return filepath.Base(args[i+1])
			}
		}
	}
	return firstNonFlagArg(args)
}

func rubyExecTitle(name string, args []string, action string) string {
	switch filepath.Base(name) {
	case "rspec":
		return "RSpec"
	case "bundle":
		return execTitle("Bundle", action)
	}
	if action == "eval" {
		return "Ruby eval"
	}
	if action == "check" {
		for i, arg := range args {
			if arg == "-c" && i+1 < len(args) {
				return execTitle("Ruby check", args[i+1])
			}
		}
		return "Ruby check"
	}
	return execTitle("Ruby", action)
}

func summarizeRubyExec(p *execPresentation, output string) {
	if p.Title == "RSpec" {
		if m := rspecSummaryRE.FindStringSubmatch(output); len(m) > 0 {
			examples, failed := atoiExec(m[1]), atoiExec(m[2])
			pending := 0
			if len(m) > 3 {
				pending = atoiExec(m[3])
			}
			parts := []string{pluralCount(examples, "example", "examples"), pluralCount(failed, "failure", "failures")}
			if pending > 0 {
				parts = append(parts, pluralCount(pending, "pending", "pending"))
			}
			p.Summary = strings.Join(parts, " · ")
			p.SuppressRaw = failed == 0
		}
	} else if p.Action == "check" && strings.Contains(output, "Syntax OK") {
		p.Summary, p.SuppressRaw = "syntax OK", true
	}
	if failures := firstFailureLines(output, 3); len(failures) > 0 {
		p.Details = failures
	}
}
