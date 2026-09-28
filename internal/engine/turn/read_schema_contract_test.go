package turn

import (
	"context"
	"slices"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin/readfile"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

func TestLoopPublishesCanonicalReadSchemaForGemini(t *testing.T) {
	handler := &recordingHandler{definition: readfile.New(nil).Definition()}
	client := &scriptedClient{
		profile: modelprofile.ResolveBuiltin("gateway", "gemini-3.8-flash", modelprofile.CatalogMetadata{}),
		streams: []scriptedStreamSpec{{events: []domain.Event{
			{Kind: domain.EventTextDelta, Text: "done"},
			{Kind: domain.EventFinish, FinishReason: domain.FinishStop},
		}}},
	}
	loop := newLoopForHandler(t, client, handler, permission.ActionAllow, permission.ModeAsk)
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect image"}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || len(client.requests[0].Tools) != 1 {
		t.Fatalf("published tools = %#v", client.requests)
	}
	published := client.requests[0].Tools[0]
	props := published.InputSchema["properties"].(map[string]any)
	view := props["view"].(map[string]any)
	values, ok := view["enum"].([]string)
	if !ok || !slices.Contains(values, "image") || slices.Contains(values, "source") {
		t.Fatalf("published read view enum = %#v", view["enum"])
	}
	for _, legacy := range []string{"query", "mode", "context"} {
		if props[legacy] != nil {
			t.Fatalf("canonical read schema leaked legacy field %q: %#v", legacy, props)
		}
	}
	if _, forbidden := published.InputSchema["additionalProperties"]; forbidden {
		t.Fatalf("Gemini schema retained unsupported additionalProperties: %#v", published.InputSchema)
	}
}
