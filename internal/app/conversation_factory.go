package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/agentprompt"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// ConversationSpec contains runtime inputs required to construct a primary
// conversation. Provider discovery and UI state stay outside this boundary.
type ConversationSpec struct {
	ProviderName    string
	ProviderType    string
	BaseURL         string
	APIKey          string
	ModelID         string
	SessionID       string
	Workspace       string
	AgentProfile    string
	ReasoningEffort sdk.ReasoningEffort
	MaxToolCalls    int
	RequestTimeout  time.Duration
	TurnTimeout     time.Duration
	RoundTimeout    time.Duration
	RemoteModel     *model.RemoteModel
}

// BuildConversation centralizes model, prompt, and turn-loop construction for
// primary inbound adapters such as TUI, ACP bootstrap, and headless mode.
func BuildConversation(service *toolcall.Service, skills *skill.Registry, agents Agents, spec ConversationSpec) (Conversation, error) {
	if service == nil {
		return nil, fmt.Errorf("build conversation: tool-call service is required")
	}
	modelID := strings.TrimSpace(spec.ModelID)
	if modelID == "" {
		return nil, nil
	}
	providerName := strings.TrimSpace(spec.ProviderName)
	if providerName == "" {
		providerName = model.DefaultProtonmanName
	}
	if !model.ProviderHasUsableAuth(providerName, spec.BaseURL, spec.APIKey) {
		return nil, nil
	}
	baseURL := model.ResolveProviderBaseURLForProtocol(providerName, spec.ProviderType, spec.BaseURL)
	clientOptions := []model.ClientOption{model.WithRequestTimeout(spec.RequestTimeout)}
	if spec.RemoteModel != nil {
		clientOptions = append(clientOptions, model.WithRemoteModelProfile(providerName, *spec.RemoteModel))
	}
	if strings.TrimSpace(spec.SessionID) != "" {
		clientOptions = append(clientOptions, model.WithSessionID(spec.SessionID))
	}
	languageModel := model.NewProviderLanguageModel(providerName, spec.ProviderType, baseURL, spec.APIKey, modelID, clientOptions...)
	agents.SetLanguageModel(languageModel)
	promptSpec, loopOptions, err := primaryConversationPolicy(spec)
	if err != nil {
		return nil, err
	}
	loopOptions = append([]turn.Option{turn.WithSystemPromptSpec(promptSpec)}, loopOptions...)
	if skills != nil {
		loopOptions = append(loopOptions, turn.WithSkillRegistry(skills))
	}
	return turn.NewLoop(languageModel, service, loopOptions...)
}

func primaryConversationPolicy(spec ConversationSpec) (agentprompt.Spec, []turn.Option, error) {
	promptSpec := agentprompt.Spec{Workspace: spec.Workspace}
	options := []turn.Option{
		turn.WithMaxToolCalls(spec.MaxToolCalls),
		turn.WithTurnTimeout(spec.TurnTimeout),
		turn.WithRoundTimeout(spec.RoundTimeout),
	}
	if profileName := strings.TrimSpace(spec.AgentProfile); profileName != "" {
		profile, err := agent.ParseProfile(profileName)
		if err != nil {
			return agentprompt.Spec{}, nil, fmt.Errorf("build conversation profile: %w", err)
		}
		promptSpec.Profile = string(profile)
		if profileSpec, ok := agent.SpecForProfile(profile); ok {
			options = append(options,
				turn.WithGroundingEvidence(profileSpec.GroundingEvidence),
				turn.WithReasoningEffort(profileSpec.Reasoning),
			)
		}
	}
	if spec.ReasoningEffort != sdk.ReasoningDefault {
		options = append(options, turn.WithExplicitReasoningEffort(spec.ReasoningEffort))
	}
	return promptSpec, options, nil
}
