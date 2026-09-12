package transcriptutil

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
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

func ToolFailureSuggestions(toolName, target string, failure *tool.Failure) []string {
	if failure == nil {
		return nil
	}
	var suggestions []string
	switch failure.Code {
	case tool.ErrorCodeNotFound:
		if strings.TrimSpace(toolName) == tool.NameRead {
			suggestions = append(suggestions, readNotFoundSuggestions(target, failure)...)
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

func readNotFoundSuggestions(target string, failure *tool.Failure) []string {
	const fallback = "inspect the parent directory before reading again"
	if failure == nil || failure.Recovery == nil ||
		failure.Recovery.Action != tool.RecoveryDiscoverResource || failure.Recovery.Tool != tool.NameLS {
		return []string{fallback}
	}
	var args struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(failure.Recovery.Arguments, &args) != nil || strings.TrimSpace(args.Path) == "" {
		return []string{fallback}
	}
	parent := strings.TrimSpace(args.Path)
	evidence := failure.RecoveryEvidence
	if evidence == nil || evidence.Action != tool.RecoveryDiscoverResource || evidence.Tool != tool.NameLS {
		return []string{fmt.Sprintf("search %q for nearby files", parent)}
	}
	suggestions := []string{fmt.Sprintf("searched %q", parent)}
	var discovered struct {
		Entries []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"entries"`
	}
	if len(evidence.StructuredOutput) == 0 || json.Unmarshal(evidence.StructuredOutput, &discovered) != nil {
		return suggestions
	}
	type candidate struct {
		name  string
		kind  string
		score int
	}
	missingName := strings.ToLower(filepath.Base(strings.TrimSpace(target)))
	missingStem := strings.TrimSuffix(missingName, filepath.Ext(missingName))
	candidates := make([]candidate, 0, len(discovered.Entries))
	for _, entry := range discovered.Entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		stem := strings.TrimSuffix(lower, filepath.Ext(lower))
		score := 0
		switch {
		case missingName != "" && lower == missingName:
			score = 100
		case missingStem != "" && stem == missingStem:
			score = 90
		case missingStem != "" && strings.HasPrefix(stem, missingStem):
			score = 80
		case missingStem != "" && strings.Contains(stem, missingStem):
			score = 70
		case missingStem != "" && strings.Contains(missingStem, stem):
			score = 60
		}
		candidates = append(candidates, candidate{name: name, kind: entry.Kind, score: score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return strings.ToLower(candidates[i].name) < strings.ToLower(candidates[j].name)
	})
	const visibleCandidates = 3
	for _, candidate := range candidates {
		if len(suggestions)-1 >= visibleCandidates {
			break
		}
		name := candidate.name
		if candidate.kind == "directory" {
			name += "/"
		}
		suggestions = append(suggestions, name)
	}
	remaining := len(candidates) - (len(suggestions) - 1)
	if remaining > 0 {
		suggestions = append(suggestions, fmt.Sprintf("+%d more", remaining))
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
