package todotool

import (
	"encoding/json"
	"strconv"
	"strings"

	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

// taskArgumentNormalizer canonicalizes stable wire variants that models produce
// for the task capability but that the published schema cannot accept: a revision
// delivered as a numeric string, and case-variant op/status spellings nested
// inside the operations array. Left unnormalized these variants fail input
// validation with a message the model cannot act on, which turns a single
// formatting slip into a repeating failure.
//
// Root-level enum fields such as action are already folded by
// tool.NormalizeArguments, which runs before this normalizer. Only spellings the
// dispatcher already accepts are canonicalized; anything else is returned
// untouched so genuine type errors still fail closed.
type taskArgumentNormalizer struct{}

// NormalizeArguments implements tool.ArgumentNormalizer. It returns nil when no
// canonicalization is needed so the caller keeps the original bytes verbatim.
func (taskArgumentNormalizer) NormalizeArguments(arguments json.RawMessage) json.RawMessage {
	object := map[string]json.RawMessage{}
	if json.Unmarshal(arguments, &object) != nil || len(object) == 0 {
		return nil
	}
	if actionName(object["action"]) != "update" {
		return nil
	}

	changed := false
	if raw, ok := object["expectedRevision"]; ok {
		if canonical, differs := canonicalRevision(raw); differs {
			object["expectedRevision"] = canonical
			changed = true
		}
	}
	if raw, ok := object["expected_revision"]; ok {
		if canonical, differs := canonicalRevision(raw); differs {
			object["expected_revision"] = canonical
			changed = true
		}
	}
	if raw, ok := object["operations"]; ok {
		if canonical, differs := canonicalOperations(raw); differs {
			object["operations"] = canonical
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return encodeObject(object)
}

func encodeObject(object map[string]json.RawMessage) json.RawMessage {
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil
	}
	return encoded
}

// actionName decodes the action field, lowercased and trimmed, or "" when the
// field is absent or not a string.
func actionName(raw json.RawMessage) string {
	var text string
	if len(raw) == 0 || json.Unmarshal(raw, &text) != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(text))
}

// canonicalRevision converts a numeric string or a padded integer into the
// integer form the schema requires. Values that are not a plain non-negative
// integer are left alone so genuine type errors still fail closed.
func canonicalRevision(raw json.RawMessage) (json.RawMessage, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, false
	}
	if trimmed[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return nil, false
		}
		trimmed = strings.TrimSpace(text)
	}
	value, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return nil, false
	}
	canonical, err := json.Marshal(value)
	if err != nil || string(canonical) == string(raw) {
		return nil, false
	}
	return canonical, true
}

// canonicalOperations rewrites each operation to its canonical spelling. The op
// kind and status are matched case-insensitively for the same reason as the
// action: dispatch accepts them, so the schema must not reject them.
func canonicalOperations(raw json.RawMessage) (json.RawMessage, bool) {
	var operations []map[string]json.RawMessage
	if json.Unmarshal(raw, &operations) != nil || operations == nil {
		return nil, false
	}
	changed := false
	for _, operation := range operations {
		if rewriteEnumField(operation, "op", canonicalPatchOp) {
			changed = true
		}
		if rewriteEnumField(operation, "status", canonicalStatus) {
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	canonical, err := json.Marshal(operations)
	if err != nil {
		return nil, false
	}
	return canonical, true
}

func rewriteEnumField(operation map[string]json.RawMessage, field string, canonical func(string) (string, bool)) bool {
	raw, ok := operation[field]
	if !ok {
		return false
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return false
	}
	value, ok := canonical(text)
	if !ok {
		return false
	}
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) == string(raw) {
		return false
	}
	operation[field] = encoded
	return true
}

func canonicalPatchOp(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(tododomain.PatchAdd):
		return string(tododomain.PatchAdd), true
	case string(tododomain.PatchSetStatus):
		return string(tododomain.PatchSetStatus), true
	case string(tododomain.PatchSetText):
		return string(tododomain.PatchSetText), true
	case string(tododomain.PatchRemove):
		return string(tododomain.PatchRemove), true
	default:
		return "", false
	}
}

func canonicalStatus(value string) (string, bool) {
	status := tododomain.Status(strings.ToLower(strings.TrimSpace(value)))
	if !status.Valid() {
		return "", false
	}
	return string(status), true
}
