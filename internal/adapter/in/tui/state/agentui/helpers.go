package agentui

import (
	"encoding/json"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func legacySubagentAction(name string) string {
	call := tool.Call{Name: strings.TrimSpace(name), Arguments: json.RawMessage(`{}`)}
	if call.Name != "subagent" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(extractStringArg(call.Arguments, "action")))
}

func extractStringArg(args json.RawMessage, key string) string {
	var values map[string]any
	if json.Unmarshal(args, &values) != nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
