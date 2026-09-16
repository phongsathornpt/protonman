package model

import (
	"context"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

// Provider-neutral model types and aliases owned by proton-domain.
type Role = domain.Role

const (
	RoleSystem    = domain.RoleSystem
	RoleUser      = domain.RoleUser
	RoleAssistant = domain.RoleAssistant
	RoleTool      = domain.RoleTool
)

type ContentPartType = domain.ContentPartType

const (
	ContentPartText  = domain.ContentPartText
	ContentPartImage = domain.ContentPartImage
)

type ContentPart = domain.ContentPart
type ToolCall = domain.ToolCall
type Message = domain.Message

func CloneMessages(messages []Message) []Message    { return domain.CloneMessages(messages) }
func EnsureMessageIDs(messages []Message) []Message { return domain.EnsureMessageIDs(messages) }
func NewMessageID() string                          { return domain.NewMessageID() }

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
		limits := domain.TokenLimits{
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
	base    port.LanguageModel
	profile modelprofile.Resolved
}

func withModelProfile(base port.LanguageModel, profile *modelprofile.Resolved) port.LanguageModel {
	if base == nil || profile == nil {
		return base
	}
	return &profiledLanguageModel{base: base, profile: cloneResolvedModelProfile(*profile)}
}

func ResolvedModelProfile(model port.LanguageModel) (modelprofile.Resolved, bool) {
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
func (m *profiledLanguageModel) Capabilities() domain.ModelCapabilities {
	return m.profile.Capabilities.Apply(m.base.Capabilities())
}
func (m *profiledLanguageModel) Metadata() domain.ModelMetadata {
	return usecase.ModelMetadataOf(m.base)
}
func (m *profiledLanguageModel) ContextWindow() int {
	return m.Metadata().TokenLimits.ContextWindow
}
func (m *profiledLanguageModel) TokenLimits() domain.TokenLimits {
	return m.Metadata().TokenLimits
}
func (m *profiledLanguageModel) Stream(ctx context.Context, request domain.Request) (port.Stream, error) {
	return m.base.Stream(ctx, request)
}
func (m *profiledLanguageModel) ResolvedModelProfile() modelprofile.Resolved {
	return cloneResolvedModelProfile(m.profile)
}

func cloneResolvedModelProfile(profile modelprofile.Resolved) modelprofile.Resolved {
	profile.Reasoning.Levels = append([]domain.ReasoningEffort(nil), profile.Reasoning.Levels...)
	return profile
}

var (
	_ port.MetadataModel = (*sessionBoundModel)(nil)
	_ port.MetadataModel = (*profiledLanguageModel)(nil)
	_ port.MetadataModel = (*lowConcurrencyModel)(nil)
	_ port.MetadataModel = (*emptyStreamRetryModel)(nil)
	_ port.MetadataModel = (*capabilityOverrideModel)(nil)
)

func (m *sessionBoundModel) Metadata() domain.ModelMetadata {
	return usecase.ModelMetadataOf(m.base)
}

func (m *lowConcurrencyModel) Metadata() domain.ModelMetadata {
	return usecase.ModelMetadataOf(m.base)
}

func (m *emptyStreamRetryModel) Metadata() domain.ModelMetadata {
	return usecase.ModelMetadataOf(m.base)
}

func (m *capabilityOverrideModel) Metadata() domain.ModelMetadata {
	metadata := usecase.ModelMetadataOf(m.base)
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
