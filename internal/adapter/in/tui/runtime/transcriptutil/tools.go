package transcriptutil

import (
	"encoding/json"
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

func ToolFailureSuggestions(toolName string, failure *tool.Failure) []string {
	if failure == nil {
		return nil
	}
	var suggestions []string
	switch failure.Code {
	case tool.ErrorCodeNotFound:
		if strings.TrimSpace(toolName) == tool.NameRead {
			suggestions = append(suggestions, readNotFoundSuggestion(failure))
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

func readNotFoundSuggestion(failure *tool.Failure) string {
	const fallback = "inspect the parent directory or discover the filename before reading again"
	if failure == nil || failure.Recovery == nil ||
		failure.Recovery.Action != tool.RecoveryDiscoverResource || failure.Recovery.Tool != tool.NameLS {
		return fallback
	}
	var args struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(failure.Recovery.Arguments, &args) != nil || strings.TrimSpace(args.Path) == "" {
		return fallback
	}
	parent := strings.TrimSpace(args.Path)
	if evidence := failure.RecoveryEvidence; evidence != nil &&
		evidence.Action == tool.RecoveryDiscoverResource && evidence.Tool == tool.NameLS {
		return fmt.Sprintf("inspected %q for nearby files; use a discovered path before reading again", parent)
	}
	return fmt.Sprintf("inspect %q or discover the filename before reading again", parent)
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
