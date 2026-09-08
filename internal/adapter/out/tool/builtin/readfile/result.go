package readfile

import (
	"encoding/json"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type artifactEnvelope struct {
	Kind        artifactKind `json:"kind"`
	Path        string       `json:"path"`
	MIMEType    string       `json:"mime_type,omitempty"`
	SizeBytes   int64        `json:"size_bytes"`
	Metadata    any          `json:"metadata,omitempty"`
	Analysis    any          `json:"analysis,omitempty"`
	Truncated   bool         `json:"truncated,omitempty"`
	Description string       `json:"description,omitempty"`
}

func artifactResult(call tool.Call, envelope artifactEnvelope, output string) (tool.Result, error) {
	structured, err := json.Marshal(envelope)
	if err != nil {
		return tool.Result{}, fmt.Errorf("encode read artifact result: %w", err)
	}
	return tool.Result{
		CallID:           call.ID,
		ToolName:         call.Name,
		Output:           output,
		StructuredOutput: structured,
		Truncated:        envelope.Truncated,
	}, nil
}
