package modelprofile

import (
	"reflect"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestPublishInputSchemaLowersGeminiOneOfToBroadObject(t *testing.T) {
	canonical := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operations": map[string]any{
				"type": "array",
				"items": map[string]any{"oneOf": []any{
					map[string]any{"type": "object", "properties": map[string]any{
						"op": map[string]any{"const": "add"}, "id": map[string]any{"type": "string"}, "text": map[string]any{"type": "string"},
					}, "required": []any{"op", "id", "text"}, "additionalProperties": false},
					map[string]any{"type": "object", "properties": map[string]any{
						"op": map[string]any{"const": "remove"}, "id": map[string]any{"type": "string"},
					}, "required": []any{"op", "id"}, "additionalProperties": false},
				}},
			},
		},
		"required":             []any{"operations"},
		"additionalProperties": false,
	}
	published := PublishInputSchema(Resolved{Compatibility: CompatibilityPolicy{ToolSchemaDialect: ToolSchemaGeminiSubset}}, canonical)
	items := published["properties"].(map[string]any)["operations"].(map[string]any)["items"].(map[string]any)
	if _, exists := items["oneOf"]; exists {
		t.Fatalf("published schema still contains oneOf: %#v", items)
	}
	props := items["properties"].(map[string]any)
	op := props["op"].(map[string]any)
	if got, want := op["enum"], []any{"add", "remove"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("op enum = %#v, want %#v", got, want)
	}
	required, _ := toAnySlice(items["required"])
	if got := stringSet(required); len(got) != 2 {
		t.Fatalf("required = %#v, want intersection [op id]", required)
	} else if _, ok := got["op"]; !ok {
		t.Fatalf("required missing op: %#v", required)
	} else if _, ok := got["id"]; !ok {
		t.Fatalf("required missing id: %#v", required)
	}
	if _, exists := canonical["additionalProperties"]; !exists {
		t.Fatal("canonical schema was mutated")
	}
}

func TestPublishInputSchemaKeepsCanonicalDialect(t *testing.T) {
	canonical := map[string]any{"type": "object", "additionalProperties": false}
	published := PublishInputSchema(Resolved{}, canonical)
	if !reflect.DeepEqual(published, canonical) {
		t.Fatalf("published = %#v, want canonical %#v", published, canonical)
	}
	published["type"] = "string"
	if canonical["type"] != "object" {
		t.Fatal("published schema mutated canonical input")
	}
}

func TestPublishInputSchemaLowersZeroArgumentToolForGemini(t *testing.T) {
	canonical := tool.NoArgumentsSchema()
	published := PublishInputSchema(Resolved{Compatibility: CompatibilityPolicy{ToolSchemaDialect: ToolSchemaGeminiSubset}}, canonical)
	if published["type"] != "object" {
		t.Fatalf("published type = %#v, want object", published["type"])
	}
	properties, ok := published["properties"].(map[string]any)
	if !ok || len(properties) != 0 {
		t.Fatalf("published properties = %#v, want empty object", published["properties"])
	}
	for _, forbidden := range []string{"additionalProperties", "oneOf", "const"} {
		if _, exists := published[forbidden]; exists {
			t.Fatalf("published zero-arg schema contains %s: %#v", forbidden, published)
		}
	}
	if !tool.IsNoArgumentsSchema(canonical) {
		t.Fatalf("canonical schema was mutated: %#v", canonical)
	}
}

func TestPublishInputSchemaPreservesUnifiedReadFileSourceFieldsForGemini(t *testing.T) {
	canonical := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":  map[string]any{"type": "string"},
			"view":  map[string]any{"type": "string", "enum": []string{"auto", "text", "source", "image", "structured", "metadata"}},
			"query": map[string]any{"type": "string"},
			"mode":  map[string]any{"type": "string", "enum": []string{"literal", "regex"}},
			"context": map[string]any{"type": "object", "properties": map[string]any{
				"before": map[string]any{"type": "integer"},
				"after":  map[string]any{"type": "integer"},
			}},
		},
		"required": []string{"path"},
	}
	published := PublishInputSchema(Resolved{Compatibility: CompatibilityPolicy{ToolSchemaDialect: ToolSchemaGeminiSubset}}, canonical)
	props := published["properties"].(map[string]any)
	view := props["view"].(map[string]any)
	values, ok := view["enum"].([]string)
	if !ok || !reflect.DeepEqual(values, []string{"auto", "text", "source", "image", "structured", "metadata"}) {
		t.Fatalf("published view enum = %#v", view["enum"])
	}
	contextSchema, ok := props["context"].(map[string]any)
	if !ok {
		t.Fatalf("published context schema = %#v", props["context"])
	}
	contextProps, ok := contextSchema["properties"].(map[string]any)
	if !ok || contextProps["before"] == nil || contextProps["after"] == nil {
		t.Fatalf("published context properties = %#v", contextSchema["properties"])
	}
	if props["query"] == nil || props["mode"] == nil {
		t.Fatalf("published source fields missing: %#v", props)
	}
}
