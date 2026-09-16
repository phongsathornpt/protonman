package memory

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
)

const maxExtractionOutputBytes = 64 * 1024

func runExtractionModel(ctx context.Context, model port.LanguageModel, transcript string) ([]candidate, error) {
	if model == nil {
		return nil, fmt.Errorf("memory extraction model is required")
	}
	request := domain.Request{
		Messages: []domain.Message{
			{ID: domain.NewMessageID(), Role: domain.RoleSystem, Content: extractionSystemPrompt},
			{ID: domain.NewMessageID(), Role: domain.RoleUser, Content: transcript},
		},
		Options: domain.ModelOptions{MaxOutputTokens: 4096, ToolChoice: domain.ToolChoiceAuto},
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
		case domain.EventTextDelta:
			if output.Len()+len(event.Text) > maxExtractionOutputBytes {
				return nil, fmt.Errorf("memory extraction output exceeds %d bytes", maxExtractionOutputBytes)
			}
			output.WriteString(event.Text)
		case domain.EventFinish:
			if event.FinishReason == domain.FinishError {
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
