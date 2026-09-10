package transcriptutil

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func JoinBody(existing, extra string) string {
	if existing == "" {
		return extra
	}
	if extra == "" {
		return existing
	}
	return existing + "\n" + extra
}

func EditPresentation(call tool.Call) (string, []string) {
	paths := call.AffectedPaths()
	summary := "editing workspace"
	if len(paths) == 1 {
		summary = "1 file"
	} else if len(paths) > 1 {
		summary = fmt.Sprintf("%d files", len(paths))
	}
	return summary, paths
}

func ToolFailureSuggestions(toolName string, code tool.ErrorCode) []string {
	var suggestions []string
	switch code {
	case tool.ErrorCodeNotFound:
		if strings.TrimSpace(toolName) == tool.NameRead {
			suggestions = append(suggestions, "ls the parent directory or find the filename")
		}
	case tool.ErrorCodeProtectedPath:
		suggestions = append(suggestions, "This path is shielded by workspace protection rules")
	case tool.ErrorCodeInternalPath:
		suggestions = append(suggestions, "Protonman internal state is reserved and unavailable to workspace tools")
	case tool.ErrorCodeOutsideWorkspace:
		suggestions = append(suggestions, "use . or a workspace-relative path")
	case tool.ErrorCodePermissionDenied:
		suggestions = append(suggestions, "Use shift+tab to cycle permission mode or allow the request")
	}
	return suggestions
}

func ExecFailureUsesExecCell(code tool.ErrorCode) bool {
	return code == tool.ErrorCodeCommandFailed || code == tool.ErrorCodeDeadlineExceeded
}

func FailureCode(result tool.Result) tool.ErrorCode {
	if result.Failure == nil {
		return ""
	}
	return result.Failure.Code
}
