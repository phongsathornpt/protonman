package modelprofile

import sdk "github.com/projectTHORN/proton/proton-sdk"

func mergeProfile(dst *Resolved, src Profile) {
	if src.Name != "" {
		dst.ProfileName = src.Name
	}
	mergeSupport(&dst.Capabilities.Tools, src.Capabilities.Tools)
	mergeSupport(&dst.Capabilities.Vision, src.Capabilities.Vision)
	mergeSupport(&dst.Capabilities.Reasoning, src.Capabilities.Reasoning)
	mergeSupport(&dst.Capabilities.ToolChoiceRequired, src.Capabilities.ToolChoiceRequired)
	mergeSupport(&dst.Reasoning.Support, src.Reasoning.Support)
	if len(src.Reasoning.Levels) > 0 {
		dst.Reasoning.Levels = append([]sdk.ReasoningEffort(nil), src.Reasoning.Levels...)
	}
	if src.Reasoning.Default != sdk.ReasoningDefault {
		dst.Reasoning.Default = src.Reasoning.Default
	}
	mergeSupport(&dst.Sampling.Temperature, src.Sampling.Temperature)
	mergeSupport(&dst.Sampling.TopP, src.Sampling.TopP)
	mergeSupport(&dst.Sampling.TopK, src.Sampling.TopK)
	if src.ContextWindow > 0 {
		dst.ContextWindow = src.ContextWindow
	}
	if len(src.PromptHints) > 0 {
		dst.PromptHints = append([]string(nil), src.PromptHints...)
	}
	if src.ToolSchemaDialect != ToolSchemaDefault {
		dst.ToolSchemaDialect = src.ToolSchemaDialect
	}
}

func mergeCatalog(dst *Resolved, src CatalogMetadata) {
	if src.Tools != nil {
		dst.Capabilities.Tools = supportFromPointer(src.Tools)
	}
	if src.Vision != nil {
		dst.Capabilities.Vision = supportFromPointer(src.Vision)
	}
	if src.ToolChoiceRequired != nil {
		dst.Capabilities.ToolChoiceRequired = supportFromPointer(src.ToolChoiceRequired)
	}
	if src.ContextWindow > 0 {
		dst.ContextWindow = src.ContextWindow
	}
	if src.Reasoning == nil {
		return
	}
	if src.Reasoning.Supported != nil {
		support := supportFromPointer(src.Reasoning.Supported)
		dst.Capabilities.Reasoning = support
		dst.Reasoning.Support = support
	}
	if len(src.Reasoning.Levels) > 0 {
		dst.Reasoning.Levels = append([]sdk.ReasoningEffort(nil), src.Reasoning.Levels...)
	}
	if src.Reasoning.Default != sdk.ReasoningDefault {
		dst.Reasoning.Default = src.Reasoning.Default
	}
}

func mergeSupport(dst *Support, src Support) {
	if src != SupportUnknown {
		*dst = src
	}
}
