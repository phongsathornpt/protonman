package model

import (
	"context"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Provider-neutral model types and aliases owned by proton-sdk.
type Role = sdk.Role

const (
	RoleSystem    = sdk.RoleSystem
	RoleUser      = sdk.RoleUser
	RoleAssistant = sdk.RoleAssistant
	RoleTool      = sdk.RoleTool
)

type ContentPartType = sdk.ContentPartType

const (
	ContentPartText  = sdk.ContentPartText
	ContentPartImage = sdk.ContentPartImage
)

type ContentPart = sdk.ContentPart
type ToolCall = sdk.ToolCall
type Message = sdk.Message

func CloneMessages(messages []Message) []Message    { return sdk.CloneMessages(messages) }
func EnsureMessageIDs(messages []Message) []Message { return sdk.EnsureMessageIDs(messages) }
func NewMessageID() string                          { return sdk.NewMessageID() }

// SnapshotMessages copies only the top-level message slice. Message payloads are
// immutable after publication, so callers can isolate append/re-slice ownership
// without duplicating content parts or tool-call argument buffers.
func SnapshotMessages(messages []Message) []Message {
	return append([]Message(nil), messages...)
}

type ResolvedRemoteMetadata struct {
	ID       string
	Name     string
	Provider string
	Features []string
	Profile  modelprofile.Resolved
}

func ResolveRemoteMetadata(providerName string, remote RemoteModel) ResolvedRemoteMetadata {
	profile := ResolveModelProfile(providerName, remote.ID, &remote)
	provider := remote.Provider
	if provider == "" {
		provider = providerName
	}
	return ResolvedRemoteMetadata{
		ID: remote.ID, Name: remote.Name, Provider: provider,
		Features: resolvedFeatureLabels(remote.Features, profile),
		Profile:  profile,
	}
}

func resolvedFeatureLabels(raw []string, profile modelprofile.Resolved) []string {
	features := make([]string, 0, len(raw)+3)
	seen := make(map[string]struct{}, len(raw)+3)
	for _, feature := range raw {
		key := strings.ToLower(strings.TrimSpace(feature))
		if key == "" || key == "tools" || key == "vision" || key == "reasoning" || key == "thinking" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		features = append(features, strings.TrimSpace(feature))
	}
	appendCapability := func(label string, support modelprofile.Support) {
		if support != modelprofile.SupportYes {
			return
		}
		if _, ok := seen[label]; ok {
			return
		}
		seen[label] = struct{}{}
		features = append(features, label)
	}
	appendCapability("tools", profile.Capabilities.Tools)
	appendCapability("vision", profile.Capabilities.Vision)
	appendCapability("reasoning", profile.Capabilities.Reasoning)
	return features
}

func WithRemoteModelProfile(providerName string, remote RemoteModel) ClientOption {
	return withResolvedModelProfile(ResolveModelProfile(providerName, remote.ID, &remote))
}

func ResolveModelProfile(providerName, modelID string, remote *RemoteModel) modelprofile.Resolved {
	metadata := modelprofile.CatalogMetadata{}
	if remote != nil {
		metadata = remote.ProfileMetadata()
		if remote.ID != "" {
			modelID = remote.ID
		}
	}
	return modelprofile.ResolveBuiltin(providerName, modelID, metadata)
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
		limits := sdk.TokenLimits{
			ContextWindow:   cloned.ContextWindow,
			MaxInputTokens:  cloned.MaxInputTokens,
			MaxOutputTokens: cloned.MaxOutputTokens,
		}
		if limits.ContextWindow > 0 || limits.MaxInputTokens > 0 || limits.MaxOutputTokens > 0 {
			c.tokenLimits = &limits
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
func (m *profiledLanguageModel) Metadata() sdk.ModelMetadata {
	return sdk.ModelMetadataOf(m.base)
}
func (m *profiledLanguageModel) ContextWindow() int {
	return m.Metadata().TokenLimits.ContextWindow
}
func (m *profiledLanguageModel) TokenLimits() sdk.TokenLimits {
	return m.Metadata().TokenLimits
}
func (m *profiledLanguageModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	return m.base.Stream(ctx, request)
}
func (m *profiledLanguageModel) ResolvedModelProfile() modelprofile.Resolved {
	return cloneResolvedModelProfile(m.profile)
}

func cloneResolvedModelProfile(profile modelprofile.Resolved) modelprofile.Resolved {
	profile.Reasoning.Levels = append([]sdk.ReasoningEffort(nil), profile.Reasoning.Levels...)
	return profile
}

var (
	_ sdk.MetadataModel = (*sessionBoundModel)(nil)
	_ sdk.MetadataModel = (*profiledLanguageModel)(nil)
	_ sdk.MetadataModel = (*lowConcurrencyModel)(nil)
	_ sdk.MetadataModel = (*emptyStreamRetryModel)(nil)
	_ sdk.MetadataModel = (*capabilityOverrideModel)(nil)
)

func (m *sessionBoundModel) Metadata() sdk.ModelMetadata {
	return sdk.ModelMetadataOf(m.base)
}

func (m *lowConcurrencyModel) Metadata() sdk.ModelMetadata {
	return sdk.ModelMetadataOf(m.base)
}

func (m *emptyStreamRetryModel) Metadata() sdk.ModelMetadata {
	return sdk.ModelMetadataOf(m.base)
}

func (m *capabilityOverrideModel) Metadata() sdk.ModelMetadata {
	metadata := sdk.ModelMetadataOf(m.base)
	limits := metadata.TokenLimits
	if m.tokenLimits != nil {
		if m.tokenLimits.ContextWindow > 0 {
			limits.ContextWindow = m.tokenLimits.ContextWindow
		}
		if m.tokenLimits.MaxInputTokens > 0 {
			limits.MaxInputTokens = m.tokenLimits.MaxInputTokens
		}
		if m.tokenLimits.MaxOutputTokens > 0 {
			limits.MaxOutputTokens = m.tokenLimits.MaxOutputTokens
		}
	}
	if m.contextWindow != nil && *m.contextWindow > 0 {
		limits.ContextWindow = *m.contextWindow
	}
	metadata.TokenLimits = limits
	return metadata
}
