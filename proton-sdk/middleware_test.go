package protonsdk

import (
	"context"
	"reflect"
	"testing"
)

type middlewareTestModel struct{ calls *[]string }

func (*middlewareTestModel) Provider() string { return "test" }
func (*middlewareTestModel) ModelID() string  { return "model" }
func (m *middlewareTestModel) Stream(context.Context, Request) (Stream, error) {
	*m.calls = append(*m.calls, "model")
	return &eventStream{events: []Event{{Kind: EventDone}}}, nil
}

func TestWrapLanguageModelOrder(t *testing.T) {
	var calls []string
	makeMiddleware := func(name string) Middleware {
		return MiddlewareFunc(func(next StreamFunc) StreamFunc {
			return func(ctx context.Context, request Request) (Stream, error) {
				calls = append(calls, name+":before")
				stream, err := next(ctx, request)
				calls = append(calls, name+":after")
				return stream, err
			}
		})
	}
	model := WrapLanguageModel(&middlewareTestModel{calls: &calls}, makeMiddleware("first"), makeMiddleware("second"))
	if model.Provider() != "test" || model.ModelID() != "model" {
		t.Fatalf("identity changed: %q/%q", model.Provider(), model.ModelID())
	}
	if _, err := model.Stream(context.Background(), Request{}); err != nil {
		t.Fatal(err)
	}
	want := []string{"first:before", "second:before", "model", "second:after", "first:after"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}
