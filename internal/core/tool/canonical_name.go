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
	"web_fetch":          {Canonical: NameWeb, Action: ActionFetch},
	"web_search":         {Canonical: NameWeb, Action: ActionSearch},
	"git_status":         {Canonical: NameGit, Action: ActionStatus},
	"write_file":         {Canonical: NameEdit, Action: ActionWrite},
	"search_replace":     {Canonical: NameEdit, Action: ActionReplace},
	"apply_patch":        {Canonical: NameEdit, Action: ActionPatch},
	"checkpoint_restore": {Canonical: NameEdit, Action: ActionRestore},
	"get_todo":           {Canonical: NameTodo, Action: ActionGet},
	"update_todo":        {Canonical: NameTodo, Action: ActionUpdate},
	"delegate_task":      {Canonical: NameSubagent, Action: ActionSpawn},
	"wait_agent":         {Canonical: NameSubagent, Action: ActionWait},
	"get_agent":          {Canonical: NameSubagent, Action: ActionGet},
	"list_agents":        {Canonical: NameSubagent, Action: ActionList},
	"cancel_agent":       {Canonical: NameSubagent, Action: ActionCancel},
	"resume_agent":       {Canonical: NameSubagent, Action: ActionResume},
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

// IsLegacyName reports whether name belongs to the compatibility-only tool namespace.
func IsLegacyName(name string) bool {
	_, ok := legacyToolAliases[strings.TrimSpace(name)]
	return ok
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
