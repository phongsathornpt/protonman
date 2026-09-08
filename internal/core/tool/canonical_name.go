package tool

import (
	"encoding/json"
	"strings"
)

type legacyToolAlias struct {
	Canonical string
	Action    string
}

var legacyToolAliases = map[string]legacyToolAlias{
	"read_file":          {Canonical: "read"},
	"list_dir":           {Canonical: "ls"},
	"find_files":         {Canonical: "find"},
	"calculate":          {Canonical: "math"},
	"web_fetch":          {Canonical: "web", Action: "fetch"},
	"git_status":         {Canonical: "git", Action: "status"},
	"write_file":         {Canonical: "edit", Action: "write"},
	"search_replace":     {Canonical: "edit", Action: "replace"},
	"apply_patch":        {Canonical: "edit", Action: "patch"},
	"checkpoint_restore": {Canonical: "edit", Action: "restore"},
}

// CanonicalName maps legacy public tool names to the current model-facing
// capability name. Unknown and external tool names pass through unchanged.
func CanonicalName(name string) string {
	name = strings.TrimSpace(name)
	if alias, ok := legacyToolAliases[name]; ok {
		return alias.Canonical
	}
	return name
}

// NormalizeLegacyArguments injects compatibility fields required by a
// capability facade while preserving already-canonical arguments unchanged.
func NormalizeLegacyArguments(name string, arguments json.RawMessage) json.RawMessage {
	alias, ok := legacyToolAliases[strings.TrimSpace(name)]
	if !ok || alias.Action == "" {
		return append(json.RawMessage(nil), arguments...)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &object); err != nil || object == nil {
		return append(json.RawMessage(nil), arguments...)
	}
	if _, exists := object["action"]; exists {
		return append(json.RawMessage(nil), arguments...)
	}
	action, err := json.Marshal(alias.Action)
	if err != nil {
		return append(json.RawMessage(nil), arguments...)
	}
	object["action"] = action
	encoded, err := json.Marshal(object)
	if err != nil {
		return append(json.RawMessage(nil), arguments...)
	}
	return encoded
}

// NormalizeLegacyCall canonicalizes a legacy call without mutating stored
// history. It is safe to apply repeatedly.
func NormalizeLegacyCall(call Call) Call {
	originalName := call.Name
	call.Name = CanonicalName(call.Name)
	call.Arguments = NormalizeLegacyArguments(originalName, call.Arguments)
	return call
}
