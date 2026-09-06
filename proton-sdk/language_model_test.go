package protonsdk

import (
	"context"
	"io"
	"testing"
)

type languageModelStub struct {
	provider string
	modelID  string
}

func (m languageModelStub) Provider() string { return m.provider }
func (m languageModelStub) ModelID() string  { return m.modelID }
func (m languageModelStub) Capabilities() ModelCapabilities {
	return ModelCapabilities{Streaming: true}
}
func (m languageModelStub) Stream(context.Context, Request) (Stream, error) {
	return emptyStream{}, nil
}

type emptyStream struct{}

func (emptyStream) Next(context.Context) (Event, error) { return Event{}, io.EOF }
func (emptyStream) Close() error                        { return nil }

func TestLanguageModelContract(t *testing.T) {
	var model LanguageModel = languageModelStub{provider: "openai", modelID: "test-model"}
	if model.Provider() != "openai" {
		t.Fatalf("Provider() = %q", model.Provider())
	}
	if model.ModelID() != "test-model" {
		t.Fatalf("ModelID() = %q", model.ModelID())
	}
	if caps := model.Capabilities(); !caps.Streaming {
		t.Fatalf("Capabilities() = %#v, want streaming", caps)
	}
	stream, err := model.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
