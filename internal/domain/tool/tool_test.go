package tool

import (
	"testing"
)

func TestNewCallCopiesArguments(t *testing.T) {
	arguments := []byte(`{"path":"README.md"}`)
	call, err := NewCall("call-1", "read_file", arguments)
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	arguments[0] = ' '
	if string(call.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("call arguments changed after source mutation: %s", call.Arguments)
	}
}

func TestNewCallDefaultsEmptyArguments(t *testing.T) {
	call, err := NewCall("call-1", "read_file", nil)
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	if string(call.Arguments) != `{}` {
		t.Fatalf("call arguments = %s, want {}", call.Arguments)
	}
}

func TestDefinitionValidateRejectsUnknownKind(t *testing.T) {
	err := (Definition{
		Name:        "broken",
		Description: "missing kind",
		Kind:        Kind("unknown"),
	}).Validate()
	if err == nil {
		t.Fatal("Definition.Validate() error = nil, want error")
	}
}
