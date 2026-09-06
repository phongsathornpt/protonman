package model

import (
	"context"

	"github.com/projectTHORN/proton/internal/modelprofile"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func WithRemoteModelProfile(providerName string, remote RemoteModel) ClientOption {
	resolved := modelprofile.ResolveBuiltin(providerName, remote.ID, remote.ProfileMetadata())
	return withResolvedModelProfile(resolved)
}

func (m RemoteModel) ProfileMetadata() modelprofile.CatalogMetadata {
	return modelprofile.CatalogMetadata{
		Tools:         m.ToolSupport,
		Vision:        m.VisionSupport,
		ContextWindow: m.ContextWindow,
		Reasoning:     m.Reasoning,
	}
}

func withResolvedModelProfile(profile modelprofile.Resolved) ClientOption {
	return func(c *clientConfig) {
		cloned := cloneResolvedModelProfile(profile)
		c.profile = &cloned
		if value, known := cloned.Capabilities.Vision.Bool(); known {
			c.vision = &value
		}
		if value, known := cloned.Capabilities.Tools.Bool(); known {
			c.tools = &value
		}
		if cloned.ContextWindow > 0 {
			window := cloned.ContextWindow
			c.contextWindow = &window
		}
	}
}

type profiledLanguageModel struct {
	base    sdk.LanguageModel
	profile modelprofile.Resolved
}

func withModelProfile(base sdk.LanguageModel, profile *modelprofile.Resolved) sdk.LanguageModel {
	if base == nil || profile == nil {
		return base
	}
	return &profiledLanguageModel{base: base, profile: cloneResolvedModelProfile(*profile)}
}

func ResolvedModelProfile(model sdk.LanguageModel) (modelprofile.Resolved, bool) {
	carrier, ok := model.(interface {
		ResolvedModelProfile() modelprofile.Resolved
	})
	if !ok {
		return modelprofile.Resolved{}, false
	}
	return carrier.ResolvedModelProfile(), true
}

func (m *profiledLanguageModel) Provider() string { return m.base.Provider() }
func (m *profiledLanguageModel) ModelID() string  { return m.base.ModelID() }
func (m *profiledLanguageModel) Capabilities() sdk.ModelCapabilities {
	return m.base.Capabilities()
}
func (m *profiledLanguageModel) ContextWindow() int {
	return sdk.ModelContextWindow(m.base)
}
func (m *profiledLanguageModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	return m.base.Stream(ctx, request)
}
func (m *profiledLanguageModel) ResolvedModelProfile() modelprofile.Resolved {
	return cloneResolvedModelProfile(m.profile)
}

func cloneResolvedModelProfile(profile modelprofile.Resolved) modelprofile.Resolved {
	profile.Reasoning.Levels = append([]sdk.ReasoningEffort(nil), profile.Reasoning.Levels...)
	profile.PromptHints = append([]string(nil), profile.PromptHints...)
	return profile
}
