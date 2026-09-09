package commandutil

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func PrefixNames(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = "/" + strings.TrimPrefix(strings.TrimSpace(name), "/")
	}
	return out
}

func ParseSubagentsEnabled(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "enable", "enabled":
		return true, nil
	case "off", "false", "disable", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("subagents must be on or off")
	}
}

func SubagentsEnabledLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func ParsePermissionRuleArgs(args []string) (permission.Rule, error) {
	if len(args) < 2 {
		return permission.Rule{}, fmt.Errorf("usage: permission <allow|deny|ask> <tool> [pattern]")
	}
	action, err := permission.ParseAction(args[0])
	if err != nil {
		return permission.Rule{}, fmt.Errorf("invalid permission action %q: use allow, deny, or ask", args[0])
	}
	toolKind, err := permission.ParseToolKind(args[1])
	if err != nil {
		return permission.Rule{}, err
	}
	patternMode := permission.PatternModeGlob
	if toolKind == permission.ToolWeb {
		patternMode = permission.PatternModeDomain
	}
	pattern := "*"
	if len(args) > 2 {
		pattern = strings.Join(args[2:], " ")
	}
	pattern = permission.NormalizePattern(toolKind, patternMode, pattern)
	return permission.Rule{Action: action, Tool: toolKind, Pattern: pattern, PatternMode: patternMode}, nil
}

func IsPermissionAction(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "allow", "deny", "ask":
		return true
	default:
		return false
	}
}
