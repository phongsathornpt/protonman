package model

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/phongsathornpt/proton/internal/core/modelprofile"
)

func decodeCompatibleModels(body []byte) ([]RemoteModel, error) {
	// Attempt parsing OpenAI format: {"data": [{"id": "model-id"}]}
	var openAIResp struct {
		Data []struct {
			ID              string   `json:"id"`
			Name            string   `json:"name"`
			ContextWindow   int      `json:"context_window"`
			ContextWindow2  int      `json:"contextWindow"`
			MaxInputTokens  int      `json:"max_input_tokens"`
			MaxOutputTokens int      `json:"max_output_tokens"`
			Provider        string   `json:"provider"`
			Features        []string `json:"features"`
			Capabilities    struct {
				Tools              *bool `json:"tools"`
				Vision             *bool `json:"vision"`
				Reasoning          *bool `json:"reasoning"`
				ToolChoiceRequired *bool `json:"tool_choice_required"`
			} `json:"capabilities"`
			Reasoning *modelprofile.CatalogReasoning `json:"reasoning"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &openAIResp); err == nil && len(openAIResp.Data) > 0 {
		results := make([]RemoteModel, 0, len(openAIResp.Data))
		for _, item := range openAIResp.Data {
			name := item.Name
			if name == "" {
				name = item.ID
			}
			reasoning := item.Reasoning
			if reasoning == nil && item.Capabilities.Reasoning != nil {
				reasoning = &modelprofile.CatalogReasoning{Supported: item.Capabilities.Reasoning}
			}
			toolSupport := item.Capabilities.Tools
			visionSupport := item.Capabilities.Vision
			if toolSupport == nil && hasModelFeature(item.Features, "tools") {
				toolSupport = boolPointer(true)
			}
			if visionSupport == nil && hasModelFeature(item.Features, "vision") {
				visionSupport = boolPointer(true)
			}
			results = append(results, RemoteModel{
				ID:                 item.ID,
				Name:               name,
				ContextWindow:      firstPositiveInt(item.ContextWindow, item.ContextWindow2),
				MaxInputTokens:     item.MaxInputTokens,
				MaxOutputTokens:    item.MaxOutputTokens,
				Provider:           item.Provider,
				Features:           item.Features,
				ToolSupport:        toolSupport,
				VisionSupport:      visionSupport,
				ToolChoiceRequired: item.Capabilities.ToolChoiceRequired,
				Reasoning:          modelprofile.NormalizeCatalogReasoning(reasoning),
			})
		}
		return results, nil
	}

	// Attempt parsing protonman format: {"models": [{"slug": "...", "name": "...", "contextWindow": 1000000}]}
	var protonmanResp struct {
		Models []struct {
			ID                         string   `json:"id"`
			Slug                       string   `json:"slug"`
			Name                       string   `json:"name"`
			ContextWindow              int      `json:"contextWindow"`
			MaxInputTokens             int      `json:"maxInputTokens"`
			MaxOutputTokens            int      `json:"maxOutputTokens"`
			Features                   []string `json:"features"`
			SupportsTools              *bool    `json:"supportsTools"`
			SupportsVision             *bool    `json:"supportsVision"`
			SupportsToolChoiceRequired *bool    `json:"supportsToolChoiceRequired"`
			Capabilities               struct {
				Tools              *bool `json:"tools"`
				Vision             *bool `json:"vision"`
				Reasoning          *bool `json:"reasoning"`
				ToolChoiceRequired *bool `json:"tool_choice_required"`
			} `json:"capabilities"`
			Reasoning *modelprofile.CatalogReasoning `json:"reasoning"`
			Provider  struct {
				Name string `json:"name"`
			} `json:"provider"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &protonmanResp); err == nil && len(protonmanResp.Models) > 0 {
		results := make([]RemoteModel, 0, len(protonmanResp.Models))
		for _, item := range protonmanResp.Models {
			id := item.Slug
			if id == "" {
				id = item.ID
			}
			toolSupport := firstKnownBool(item.Capabilities.Tools, item.SupportsTools)
			visionSupport := firstKnownBool(item.Capabilities.Vision, item.SupportsVision)
			requiredToolChoice := firstKnownBool(item.Capabilities.ToolChoiceRequired, item.SupportsToolChoiceRequired)
			if toolSupport == nil && hasModelFeature(item.Features, "tools") {
				toolSupport = boolPointer(true)
			}
			if visionSupport == nil && hasModelFeature(item.Features, "vision") {
				visionSupport = boolPointer(true)
			}
			reasoning := item.Reasoning
			if reasoning == nil && item.Capabilities.Reasoning != nil {
				reasoning = &modelprofile.CatalogReasoning{Supported: item.Capabilities.Reasoning}
			}
			results = append(results, RemoteModel{
				ID:                 id,
				Name:               item.Name,
				ContextWindow:      item.ContextWindow,
				MaxInputTokens:     item.MaxInputTokens,
				MaxOutputTokens:    item.MaxOutputTokens,
				Provider:           item.Provider.Name,
				Features:           item.Features,
				ToolSupport:        toolSupport,
				VisionSupport:      visionSupport,
				ToolChoiceRequired: requiredToolChoice,
				Reasoning:          modelprofile.NormalizeCatalogReasoning(reasoning),
			})
		}
		return results, nil
	}

	return nil, errors.New("unrecognized models response format")
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func boolPointer(value bool) *bool { return &value }

func firstKnownBool(values ...*bool) *bool {
	for _, value := range values {
		if value != nil {
			copy := *value
			return &copy
		}
	}
	return nil
}

func hasModelFeature(features []string, want string) bool {
	for _, feature := range features {
		if strings.EqualFold(strings.TrimSpace(feature), want) {
			return true
		}
	}
	return false
}
