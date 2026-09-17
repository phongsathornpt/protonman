package openai

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func TestChatStreamRepairsMultilineToolArgumentsAtCompletion(t *testing.T) {
	// The escaped newlines below become literal newlines when the SSE payload is
	// decoded into the provider's tool-call argument string.
	body := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"edit\",\"arguments\":\"{\\\"oldString\\\":\\\"line one\\nline two\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n"
	stream := newStream(io.NopCloser(strings.NewReader(body)), nil, false, "test")
	defer stream.Close()

	var complete bool
	for {
		event, err := stream.Next(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := event.Validate(); err != nil {
			t.Fatal(err)
		}
		if event.Kind == domain.EventToolCall {
			complete = true
			if string(event.ToolCall.Arguments) != `{"oldString":"line one\nline two"}` {
				t.Fatalf("arguments = %q", event.ToolCall.Arguments)
			}
		}
	}
	if !complete {
		t.Fatal("missing complete tool call")
	}
}

func TestResponsesStreamRepairsMultilineToolArgumentsAtCompletion(t *testing.T) {
	body := "data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"item-1\",\"type\":\"function_call\",\"call_id\":\"call-1\",\"name\":\"edit\",\"arguments\":\"{\\\"oldString\\\":\\\"line one\\nline two\\\"}\"}}\n\n" +
		"data: {\"type\":\"response.completed\"}\n\n"
	stream := newStream(io.NopCloser(strings.NewReader(body)), nil, false, "test")
	defer stream.Close()

	for {
		event, err := stream.Next(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := event.Validate(); err != nil {
			t.Fatal(err)
		}
		if event.Kind == domain.EventToolCall {
			if string(event.ToolCall.Arguments) != `{"oldString":"line one\nline two"}` {
				t.Fatalf("arguments = %q", event.ToolCall.Arguments)
			}
			return
		}
	}
	t.Fatal("missing complete tool call")
}

func TestNormalizeToolArgumentsEscapesLiteralStringControls(t *testing.T) {
	raw := "{\"filePath\":\"main.go\",\"oldString\":\"line one\nline two\",\"newString\":\"line one\tupdated\"}"
	got, repaired := normalizeToolArguments(raw)
	if !repaired {
		t.Fatal("expected literal controls to be repaired")
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("normalized arguments are invalid JSON: %q", got)
	}
	var decoded struct {
		OldString string `json:"oldString"`
		NewString string `json:"newString"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OldString != "line one\nline two" || decoded.NewString != "line one\tupdated" {
		t.Fatalf("decoded arguments = %#v", decoded)
	}
}

func TestNormalizeToolArgumentsPreservesCRLFValue(t *testing.T) {
	raw := "{\"oldString\":\"line one\r\nline two\"}"
	got, repaired := normalizeToolArguments(raw)
	if !repaired {
		t.Fatal("expected literal CRLF to be repaired")
	}
	var decoded struct {
		OldString string `json:"oldString"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OldString != "line one\r\nline two" {
		t.Fatalf("decoded oldString = %q", decoded.OldString)
	}
}

func TestNormalizeToolArgumentsPreservesValidJSON(t *testing.T) {
	want := `{"filePath":"main.go","oldString":"line\nline"}`
	got, repaired := normalizeToolArguments(want)
	if repaired || got != want {
		t.Fatalf("normalized valid arguments = %q, want %q", got, want)
	}
}

func TestNormalizeToolArgumentsDoesNotRepairStructuralJSON(t *testing.T) {
	want := `{"filePath":"main.go",}`
	got, repaired := normalizeToolArguments(want)
	if repaired || got != want {
		t.Fatalf("normalized structural error = %q, want unchanged %q", got, want)
	}
}
