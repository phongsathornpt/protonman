package modelcatalog

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
)

func TestRemoteModelProfileMetadata(t *testing.T) {
	tools := true
	vision := false
	toolChoice := true
	supported := true
	reasoning := &modelprofile.CatalogReasoning{
		Supported: &supported,
	}

	model := RemoteModel{
		ID:                 "test-model-1",
		Name:               "Test Model 1",
		ContextWindow:      128000,
		MaxInputTokens:     64000,
		MaxOutputTokens:    4096,
		Provider:           "openai",
		Features:           []string{"chat", "tools"},
		ToolSupport:        &tools,
		VisionSupport:      &vision,
		ToolChoiceRequired: &toolChoice,
		Reasoning:          reasoning,
	}

	meta := model.ProfileMetadata()
	if meta.ContextWindow != 128000 {
		t.Errorf("ContextWindow = %d; want 128000", meta.ContextWindow)
	}
	if meta.MaxInputTokens != 64000 {
		t.Errorf("MaxInputTokens = %d; want 64000", meta.MaxInputTokens)
	}
	if meta.MaxOutputTokens != 4096 {
		t.Errorf("MaxOutputTokens = %d; want 4096", meta.MaxOutputTokens)
	}
	if meta.Tools == nil || *meta.Tools != true {
		t.Errorf("Tools = %v; want true", meta.Tools)
	}
	if meta.Vision == nil || *meta.Vision != false {
		t.Errorf("Vision = %v; want false", meta.Vision)
	}
	if meta.ToolChoiceRequired == nil || *meta.ToolChoiceRequired != true {
		t.Errorf("ToolChoiceRequired = %v; want true", meta.ToolChoiceRequired)
	}
	if meta.Reasoning == nil || meta.Reasoning.Supported == nil || !*meta.Reasoning.Supported {
		t.Errorf("Reasoning = %v; want supported", meta.Reasoning)
	}
}

func TestRemoteModelProfileMetadataDefaults(t *testing.T) {
	model := RemoteModel{
		ID:   "minimal-model",
		Name: "Minimal Model",
	}

	meta := model.ProfileMetadata()
	if meta.ContextWindow != 0 {
		t.Errorf("ContextWindow = %d; want 0", meta.ContextWindow)
	}
	if meta.Tools != nil {
		t.Errorf("Tools = %v; want nil", meta.Tools)
	}
	if meta.Vision != nil {
		t.Errorf("Vision = %v; want nil", meta.Vision)
	}
	if meta.Reasoning != nil {
		t.Errorf("Reasoning = %v; want nil", meta.Reasoning)
	}
}
