package tui

import "strings"

func makeAction(args []string) string {
	for _, arg := range args {
		if arg == "--" {
			continue
		}
		if !strings.HasPrefix(arg, "-") && !strings.Contains(arg, "=") {
			return arg
		}
	}
	return ""
}

func makeExecTitle(_ string, _ []string, action string) string {
	return execTitle("Make", action)
}

func summarizeMakeExec(p *execPresentation, output string) {
	if strings.TrimSpace(output) == "" {
		p.SuccessSummary = "completed"
		return
	}
	if failures := firstFailureLines(output, 3); len(failures) > 0 {
		p.Details = failures
	}
}
