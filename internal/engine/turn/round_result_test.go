package turn

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestToolMessagesForExecutionsVisionGating(t *testing.T) {
	executions := []executedCall{
		{
			call: tool.Call{ID: "call-1", Name: "read"},
			result: tool.Result{
				CallID:   "call-1",
				ToolName: "read",
				Output:   "image summary",
				Image: &tool.ImageAttachment{
					MIMEType: "image/png",
					Data:     "iVBORw0KGgo=",
					Width:    100,
					Height:   100,
				},
			},
		},
	}

	// 1. Vision enabled: message should have Parts with ContentPartImage
	withVision, err := toolMessagesForExecutions(executions, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(withVision) != 1 {
		t.Fatalf("expected 1 message, got %d", len(withVision))
	}
	if len(withVision[0].Parts) != 2 {
		t.Fatalf("expected 2 parts (text + image), got %#v", withVision[0].Parts)
	}
	if withVision[0].Parts[0].Type != sdk.ContentPartText {
		t.Fatalf("expected part 0 to be text, got %#v", withVision[0].Parts[0])
	}
	if withVision[0].Parts[1].Type != sdk.ContentPartImage || withVision[0].Parts[1].MIMEType != "image/png" || withVision[0].Parts[1].Data != "iVBORw0KGgo=" {
		t.Fatalf("expected part 1 to be image, got %#v", withVision[0].Parts[1])
	}

	// 2. Vision disabled: message should have NO Parts, only Content string
	withoutVision, err := toolMessagesForExecutions(executions, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(withoutVision) != 1 {
		t.Fatalf("expected 1 message, got %d", len(withoutVision))
	}
	if len(withoutVision[0].Parts) != 0 {
		t.Fatalf("expected 0 parts when vision is disabled, got %#v", withoutVision[0].Parts)
	}
	if withoutVision[0].Content == "" {
		t.Fatal("expected non-empty Content string")
	}
}
