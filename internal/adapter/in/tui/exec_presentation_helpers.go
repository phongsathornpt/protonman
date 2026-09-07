package tui

import (
	"fmt"
	"strings"
	"time"
)

func nonEmptyExecLines(value string) []string {
	raw := strings.Split(strings.TrimSpace(value), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func capExecLines(lines []string, limit int) []string {
	if len(lines) <= limit {
		return append([]string(nil), lines...)
	}
	out := append([]string(nil), lines[:limit]...)
	out = append(out, fmt.Sprintf("… %d more", len(lines)-limit))
	return out
}

func tailExecLines(lines []string, limit int) []string {
	if len(lines) <= limit {
		return append([]string(nil), lines...)
	}
	return append([]string(nil), lines[len(lines)-limit:]...)
}

func filterExecLines(value string, keep func(string) bool, limit int) []string {
	out := make([]string, 0, limit)
	for _, line := range nonEmptyExecLines(value) {
		if keep(line) {
			out = append(out, line)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

func pluralCount(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

func formatExecDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Round(time.Millisecond)/time.Millisecond)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm %02ds", minutes, seconds)
}

type testCounts struct {
	Passed  int
	Failed  int
	Skipped int
	Ignored int
	Errors  int
}

type diagnosticCounts struct {
	Errors   int
	Warnings int
}

type changeCounts struct {
	Added     int
	Changed   int
	Destroyed int
}

func formatTestCounts(c testCounts) string {
	parts := make([]string, 0, 5)
	if c.Passed > 0 {
		parts = append(parts, pluralCount(c.Passed, "passed", "passed"))
	}
	if c.Failed > 0 {
		parts = append(parts, pluralCount(c.Failed, "failed", "failed"))
	}
	if c.Errors > 0 {
		parts = append(parts, pluralCount(c.Errors, "error", "errors"))
	}
	if c.Skipped > 0 {
		parts = append(parts, pluralCount(c.Skipped, "skipped", "skipped"))
	}
	if c.Ignored > 0 {
		parts = append(parts, pluralCount(c.Ignored, "ignored", "ignored"))
	}
	return strings.Join(parts, " · ")
}

func formatDiagnosticCounts(c diagnosticCounts) string {
	parts := make([]string, 0, 2)
	if c.Errors > 0 {
		parts = append(parts, pluralCount(c.Errors, "error", "errors"))
	}
	if c.Warnings > 0 {
		parts = append(parts, pluralCount(c.Warnings, "warning", "warnings"))
	}
	return strings.Join(parts, " · ")
}

func firstFailureLines(output string, limit int) []string {
	return filterExecLines(output, func(line string) bool {
		lower := strings.ToLower(strings.TrimSpace(line))
		return strings.Contains(lower, "fail") || strings.Contains(lower, "error") || strings.HasPrefix(lower, "panic:")
	}, limit)
}

func firstNonFlagArg(args []string) string {
	for _, arg := range args {
		if arg != "" && !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}
