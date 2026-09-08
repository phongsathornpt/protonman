package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/phongsathornpt/proton/internal/core/tool"
)

var unmergedGitStatusCodes = map[string]struct{}{
	"DD": {}, "AU": {}, "UD": {}, "UA": {}, "DU": {}, "AA": {}, "UU": {},
}

func (h bashHandler) gitConflictPaths(ctx context.Context, cwd string) ([]string, error) {
	command, err := h.command(ctx, cwd, "git -c core.hooksPath=/dev/null --no-optional-locks status --porcelain=v1 -z --untracked-files=no")
	if err != nil {
		return nil, fmt.Errorf("prepare git conflict status: %w", err)
	}
	stdout := boundedBuffer{limit: maxBashStreamBytes}
	stderr := boundedBuffer{limit: maxBashStreamBytes}
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return nil, fmt.Errorf("inspect git conflict status: %w", err)
		}
		return nil, fmt.Errorf("inspect git conflict status: %w: %s", err, detail)
	}
	if stdout.IsTruncated() {
		return nil, fmt.Errorf("inspect git conflict status: output exceeded limit")
	}
	return parseGitConflictPaths(stdout.String()), nil
}

func parseGitConflictPaths(output string) []string {
	records := strings.Split(output, "\x00")
	paths := make([]string, 0)
	for _, record := range records {
		if len(record) < 4 || record[2] != ' ' {
			continue
		}
		if _, ok := unmergedGitStatusCodes[record[:2]]; !ok {
			continue
		}
		path := strings.TrimSpace(record[3:])
		if path == "" {
			continue
		}
		paths = appendUniqueString(paths, path)
	}
	return paths
}

func appendUniqueString(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func conflictSummary(paths []string) string {
	if len(paths) == 0 {
		return "git operation left unresolved conflicts"
	}
	const maxPaths = 8
	visible := paths
	if len(visible) > maxPaths {
		visible = visible[:maxPaths]
	}
	message := "git operation left unresolved conflicts: " + strings.Join(visible, ", ")
	if len(paths) > len(visible) {
		message += fmt.Sprintf(" (+%d more)", len(paths)-len(visible))
	}
	return message
}

func (h bashHandler) markConflictPathsOwned(ctx context.Context, cwdRel string, paths []string) {
	if h.workspace == nil || len(paths) == 0 {
		return
	}
	workspacePaths := bashAffectedPaths(cwdRel, paths)
	resolved := make([]string, 0, len(workspacePaths))
	for _, path := range workspacePaths {
		absolute, err := h.workspace.Resolve(ctx, path)
		if err != nil {
			continue
		}
		resolved = append(resolved, absolute)
	}
	h.workspace.MarkMutationOwned(ctx, resolved...)
}

func gitConflictError(paths []string) error {
	return tool.NewToolError(tool.ErrorCodeConflict, conflictSummary(paths))
}
