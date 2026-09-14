package readfile

import (
	"context"
	"encoding/base64"
	"image"
	"io"
	"os"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/imageprep"
)

// VisionTokenEstimate is retained as compatibility metadata for read(view=image).
// Both fields now carry the provider-neutral 32px patch estimate; request-level
// provider accounting belongs to the turn context layer, not the read tool.
type VisionTokenEstimate struct {
	OpenAI    int `json:"openai"`
	Anthropic int `json:"anthropic"`
}

func visionTargetDimensions(width, height int) (int, int) {
	return imageprep.OutputDimensions(width, height, imageprep.DefaultPolicy())
}

func estimateVisionTokens(width, height int) VisionTokenEstimate {
	if width <= 0 || height <= 0 {
		return VisionTokenEstimate{}
	}
	patch := imageprep.PatchSize
	patchesWide := (width + patch - 1) / patch
	patchesHigh := (height + patch - 1) / patch
	count := int(int64(patchesWide) * int64(patchesHigh))
	if count > imageprep.OriginalMaxPatches {
		count = imageprep.OriginalMaxPatches
	}
	if count < 1 {
		count = 1
	}
	return VisionTokenEstimate{OpenAI: count, Anthropic: count}
}

// prepareVisionAttachment is now a thin adapter around the shared prompt-image
// pipeline. It intentionally contains no private resize/encoding policy.
func prepareVisionAttachment(ctx context.Context, file *os.File, info os.FileInfo, _ image.Image, _ string, mimeType string) (*tool.ImageAttachment, VisionTokenEstimate) {
	if err := ctx.Err(); err != nil || file == nil || info == nil {
		return nil, VisionTokenEstimate{}
	}
	if info.Size() < 0 || info.Size() > imageprep.MaxSnapshotBytes {
		return nil, VisionTokenEstimate{}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, VisionTokenEstimate{}
	}
	raw, err := io.ReadAll(io.LimitReader(file, imageprep.MaxSnapshotBytes+1))
	if err != nil || len(raw) > imageprep.MaxSnapshotBytes {
		return nil, VisionTokenEstimate{}
	}
	if err := ctx.Err(); err != nil {
		return nil, VisionTokenEstimate{}
	}
	prepared, err := imageprep.Prepare(mimeType, base64.StdEncoding.EncodeToString(raw), imageprep.DefaultPolicy())
	if err != nil {
		return nil, VisionTokenEstimate{}
	}
	estimate := estimateVisionTokens(prepared.Width, prepared.Height)
	return &tool.ImageAttachment{
		MIMEType: prepared.MIMEType,
		Data:     prepared.Data,
		Width:    prepared.Width,
		Height:   prepared.Height,
	}, estimate
}
