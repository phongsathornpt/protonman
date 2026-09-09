package agentui

import (
	"encoding/json"
	"strings"
)

func extractStringArg(args json.RawMessage, key string) string {
	var values map[string]any
	if json.Unmarshal(args, &values) != nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
