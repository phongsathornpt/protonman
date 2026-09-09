package history

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func resultBodyLines(body string, exitCode *int, truncated bool, denied bool, failureCode tool.ErrorCode) []string {
	body = strings.TrimRight(body, "\n")
	parts := make([]string, 0, strings.Count(body, "\n")+4)
	if body != "" {
		parts = append(parts, rawTextLines(body)...)
	}
	if exitCode != nil {
		parts = append(parts, fmt.Sprintf("exit %d", *exitCode))
	}
	if truncated {
		parts = append(parts, "output truncated")
	}
	if denied {
		parts = append(parts, "denied")
	}
	if failureCode != "" {
		parts = append(parts, "failure: "+string(failureCode))
	}
	return parts
}
