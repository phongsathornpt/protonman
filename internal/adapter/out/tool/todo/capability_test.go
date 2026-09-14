package todotool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// capabilityValidator compiles the exact schema the model is shown, so these
// tests exercise the published contract rather than a hand-built copy.
func capabilityValidator(t *testing.T) *sdk.ToolSchemaValidator {
	t.Helper()
	definition := NewTodo(nil).Definition()
	validator, err := sdk.CompileToolInputValidator(sdk.Tool{
		Name:        definition.Name,
		Description: definition.Description,
		InputSchema: definition.InputSchema,
	})
	if err != nil {
		t.Fatalf("compile task capability schema: %v", err)
	}
	return validator
}

// normalizeThroughCapability applies the same handler-normalization step the
// tool-call service performs before schema validation.
func normalizeThroughCapability(t *testing.T, args string) json.RawMessage {
	t.Helper()
	store, err := tododomain.NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewTodo(store)
	return tool.NormalizeArgumentsForHandler(handler, handler.Definition(), json.RawMessage(args))
}

func TestTodoCapabilityAcceptsCanonicalCalls(t *testing.T) {
	validator := capabilityValidator(t)
	calls := map[string]string{
		"get":                      `{"action":"get"}`,
		"update add":               `{"action":"update","expectedRevision":0,"operations":[{"op":"add","id":"a","text":"t","status":"pending"}]}`,
		"update set_status":        `{"action":"update","expectedRevision":0,"operations":[{"op":"set_status","id":"a","status":"completed"}]}`,
		"update set_text":          `{"action":"update","expectedRevision":0,"operations":[{"op":"set_text","id":"a","text":"x"}]}`,
		"update remove":            `{"action":"update","expectedRevision":0,"operations":[{"op":"remove","id":"a"}]}`,
		"update mixed operations":  `{"action":"update","expectedRevision":0,"operations":[{"op":"add","id":"a","text":"t","status":"pending"},{"op":"set_status","id":"a","status":"in_progress"},{"op":"remove","id":"a"}]}`,
		"get with echoed revision": `{"action":"get","expectedRevision":0}`,
	}
	for name, args := range calls {
		if err := validator.Validate(normalizeThroughCapability(t, args)); err != nil {
			t.Errorf("%s rejected: %v", name, err)
		}
	}
}

// These are the wire variants that previously failed validation with a message
// the model could not act on.
func TestTodoCapabilityAcceptsNormalizedWireVariants(t *testing.T) {
	validator := capabilityValidator(t)
	calls := map[string]string{
		"quoted revision":            `{"action":"update","expectedRevision":"7","operations":[{"op":"remove","id":"a"}]}`,
		"echoed session_id":          `{"action":"update","sessionId":"session-1","expectedRevision":0,"operations":[{"op":"remove","id":"a"}]}`,
		"uppercase action":           `{"action":"UPDATE","expectedRevision":0,"operations":[{"op":"remove","id":"a"}]}`,
		"uppercase op":               `{"action":"update","expectedRevision":0,"operations":[{"op":"REMOVE","id":"a"}]}`,
		"uppercase status":           `{"action":"update","expectedRevision":0,"operations":[{"op":"add","id":"a","text":"t","status":"PENDING"}]}`,
		"echoed revision with get":   `{"action":"get","sessionId":"session-1","expectedRevision":0}`,
		"quoted revision and echoes": `{"action":"update","sessionId":"s","expectedRevision":"2","operations":[{"op":"SET_STATUS","id":"a","status":"COMPLETED"}]}`,
	}
	for name, args := range calls {
		if err := validator.Validate(normalizeThroughCapability(t, args)); err != nil {
			t.Errorf("%s rejected: %v", name, err)
		}
	}
}

func TestTodoCapabilityStillRejectsGenuineMisuse(t *testing.T) {
	validator := capabilityValidator(t)
	calls := map[string]string{
		"missing action":            `{}`,
		"unknown action":            `{"action":"delete"}`,
		"update without revision":   `{"action":"update","operations":[{"op":"remove","id":"a"}]}`,
		"update without operations": `{"action":"update","expectedRevision":0}`,
		"update with no operations": `{"action":"update","expectedRevision":0,"operations":[]}`,
		"non-numeric revision":      `{"action":"update","expectedRevision":"abc","operations":[{"op":"remove","id":"a"}]}`,
		"negative revision":         `{"action":"update","expectedRevision":-1,"operations":[{"op":"remove","id":"a"}]}`,
		"unknown op":                `{"action":"update","expectedRevision":0,"operations":[{"op":"nope","id":"a"}]}`,
		"unknown status":            `{"action":"update","expectedRevision":0,"operations":[{"op":"add","id":"a","text":"t","status":"nope"}]}`,
		"operation without op":      `{"action":"update","expectedRevision":0,"operations":[{"id":"a"}]}`,
		"unknown top-level key":     `{"action":"get","unexpected":1}`,
	}
	for name, args := range calls {
		if err := validator.Validate(normalizeThroughCapability(t, args)); err == nil {
			t.Errorf("%s was accepted, want rejection", name)
		}
	}
}

// A schema failure must name the offending field so the model can correct it
// within the compaction budget applied to model-visible failures.
func TestTodoCapabilitySchemaErrorIsActionable(t *testing.T) {
	validator := capabilityValidator(t)
	err := validator.Validate(normalizeThroughCapability(t, `{"action":"update","operations":[{"op":"remove","id":"a"}]}`))
	if err == nil {
		t.Fatal("missing expectedRevision was accepted")
	}
	if !strings.Contains(err.Error(), "expectedRevision") {
		t.Fatalf("schema error does not name the offending field: %v", err)
	}
}

// The get branch rejects a real patch attempt, and the facade reports it with an
// actionable message instead of a generic schema failure.
func TestTodoGetRejectsPatchIntent(t *testing.T) {
	store, err := tododomain.NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewTodo(store)
	_, _, err = handler.(todoHandler).resolve(json.RawMessage(`{"action":"get","operations":[{"op":"remove","id":"a"}]}`))
	if err == nil {
		t.Fatal("get with operations was accepted")
	}
	if !strings.Contains(err.Error(), "action=update") {
		t.Fatalf("misuse error is not actionable: %v", err)
	}
}

func TestTodoPermissionDetailDescribesTaskPatch(t *testing.T) {
	store, err := tododomain.NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewTodo(store)
	detail := handler.(todoHandler).PermissionDetail(json.RawMessage(`{"action":"update","expectedRevision":3,"operations":[{"op":"remove","id":"a"},{"op":"remove","id":"b"}]}`))
	if !strings.Contains(detail, "2 task operations") || !strings.Contains(detail, "revision 3") {
		t.Fatalf("permission detail = %q", detail)
	}
	// A malformed action must still yield a non-empty description so the
	// approval prompt explains what was attempted.
	for _, args := range []string{`{"action":"update"}`, `{"action":"update","operations":"not-an-array"}`, `{"unexpected":true}`} {
		if got := strings.TrimSpace(handler.(todoHandler).PermissionDetail(json.RawMessage(args))); got == "" {
			t.Fatalf("permission detail for %s is empty", args)
		}
	}
}
