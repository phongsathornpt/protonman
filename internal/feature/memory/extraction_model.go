package memory

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const maxExtractionOutputBytes = 64 * 1024

func runExtractionModel(ctx context.Context, model sdk.LanguageModel, transcript string) ([]candidate, error) {
	if model == nil {
		return nil, fmt.Errorf("memory extraction model is required")
	}
	request := sdk.Request{
		Messages: []sdk.Message{
			{ID: sdk.NewMessageID(), Role: sdk.RoleSystem, Content: extractionSystemPrompt},
			{ID: sdk.NewMessageID(), Role: sdk.RoleUser, Content: transcript},
		},
		Options: sdk.ModelOptions{MaxOutputTokens: 4096, ToolChoice: sdk.ToolChoiceAuto},
	}
	stream, err := model.Stream(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("start memory extraction: %w", err)
	}
	defer stream.Close()
	var output strings.Builder
	for {
		event, nextErr := stream.Next(ctx)
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				break
			}
			return nil, fmt.Errorf("read memory extraction stream: %w", nextErr)
		}
		switch event.Kind {
		case sdk.EventTextDelta:
			if output.Len()+len(event.Text) > maxExtractionOutputBytes {
				return nil, fmt.Errorf("memory extraction output exceeds %d bytes", maxExtractionOutputBytes)
			}
			output.WriteString(event.Text)
		case sdk.EventFinish:
			if event.FinishReason == sdk.FinishError {
				return nil, fmt.Errorf("memory extraction model finished with error")
			}
			return parseExtractionOutput(output.String())
		}
	}
	if strings.TrimSpace(output.String()) == "" {
		return nil, fmt.Errorf("memory extraction returned empty output")
	}
	return parseExtractionOutput(output.String())
}
