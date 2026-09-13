package protonsdk

// RequestRequirements describes provider-neutral capabilities required to
// execute one canonical SDK request. The Go SDK exposes streaming as its model
// execution contract, so Streaming is always required for Request values.
//
// This mirrors the request-requirement boundary used by proton-sdk-rs while
// keeping the shape limited to capabilities the Go SDK can currently publish
// authoritatively.
type RequestRequirements struct {
	Streaming       bool
	Tools           bool
	Vision          bool
	ProviderOptions bool
	RawChunks       bool
}

// Requirements derives the effective model capabilities required by the
// request before provider-specific lowering.
func (r Request) Requirements() RequestRequirements {
	requirements := RequestRequirements{
		Streaming:       true,
		ProviderOptions: len(r.Options.ProviderOptions) > 0,
		RawChunks:       r.Options.IncludeRawChunks,
		Tools:           len(r.Tools) > 0 || r.Options.ToolChoice == ToolChoiceRequired,
	}

	for _, tool := range r.Tools {
		if len(tool.ProviderOptions) > 0 {
			requirements.ProviderOptions = true
		}
	}

	for _, message := range r.Messages {
		if message.Role == RoleTool || len(message.ToolCalls) > 0 {
			requirements.Tools = true
		}
		for _, part := range message.Parts {
			if part.Type == ContentPartImage {
				requirements.Vision = true
			}
		}
	}

	return requirements
}
