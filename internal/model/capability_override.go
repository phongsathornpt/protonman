package model

import (
	"context"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

type capabilityOverrideModel struct {
	base   sdk.LanguageModel
	vision *bool
	tools  *bool
}

func withVisionCapability(base sdk.LanguageModel, vision bool) sdk.LanguageModel {
	return &capabilityOverrideModel{base: base, vision: &vision}
}

func withToolsCapability(base sdk.LanguageModel, tools bool) sdk.LanguageModel {
	return &capabilityOverrideModel{base: base, tools: &tools}
}

func (m *capabilityOverrideModel) Provider() string { return m.base.Provider() }
func (m *capabilityOverrideModel) ModelID() string  { return m.base.ModelID() }
func (m *capabilityOverrideModel) Capabilities() sdk.ModelCapabilities {
	caps := m.base.Capabilities()
	if m.vision != nil {
		caps.Vision = *m.vision
	}
	if m.tools != nil {
		caps.Tools = *m.tools
	}
	return caps
}
func (m *capabilityOverrideModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	return m.base.Stream(ctx, request)
}
