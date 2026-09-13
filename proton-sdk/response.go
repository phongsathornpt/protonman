package protonsdk

// Response is the normalized output collected from one model stream.
type Response struct {
	Text             string
	ToolCalls        []ToolCall
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata ProviderMetadata
}

// StepResult is retained for source compatibility with earlier SDK releases.
// Deprecated: use Response.
type StepResult = Response
