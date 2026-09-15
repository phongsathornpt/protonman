package readfile

import (
	"strings"
	"testing"
)

func TestReadDefinitionDiscouragesRedundantAttachedImageInspection(t *testing.T) {
	definition := (readFileHandler{}).Definition()
	description := strings.ToLower(definition.Description)
	for _, want := range []string{"attached", "current user message", "directly visible", "not be re-read"} {
		if !strings.Contains(description, want) {
			t.Fatalf("read description %q does not contain %q", definition.Description, want)
		}
	}
}

func TestReadImageViewTargetsWorkspaceImagesNotCurrentAttachments(t *testing.T) {
	definition := (readFileHandler{}).Definition()
	properties, ok := definition.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("read input schema properties missing")
	}
	view, ok := properties["view"].(map[string]any)
	if !ok {
		t.Fatal("read view schema missing")
	}
	description, _ := view["description"].(string)
	if !strings.Contains(strings.ToLower(description), "not already supplied as a current-message attachment") {
		t.Fatalf("view description = %q, want current-message attachment guidance", description)
	}
}
