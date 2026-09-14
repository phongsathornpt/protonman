package readfile

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"golang.org/x/image/draw"
)

const (
	defaultMaxVisionDimension = 2048
	defaultMaxVisionPatches   = 2500
	visionPatchSize           = 32
	maxVisionAttachBytes      = 10 * 1024 * 1024 // 10 MiB limit for base64 attachment
)

func visionPatchCount(w, h int) int {
	patchesW := (w + visionPatchSize - 1) / visionPatchSize
	patchesH := (h + visionPatchSize - 1) / visionPatchSize
	return patchesW * patchesH
}

func visionTargetDimensions(width, height int, maxDim int, maxPatches int) (int, int) {
	if width <= 0 || height <= 0 {
		return 1, 1
	}
	if width <= maxDim && height <= maxDim && visionPatchCount(width, height) <= maxPatches {
		return width, height
	}
	scale := 1.0
	maxSide := max(width, height)
	if maxSide > maxDim {
		scale = float64(maxDim) / float64(maxSide)
	}
	curW := max(1, int(math.Round(float64(width)*scale)))
	curH := max(1, int(math.Round(float64(height)*scale)))
	if visionPatchCount(curW, curH) <= maxPatches {
		return curW, curH
	}
	patchBudgetArea := float64(maxPatches * visionPatchSize * visionPatchSize)
	curArea := float64(curW * curH)
	scale2 := math.Sqrt(patchBudgetArea / curArea)
	curW = max(1, int(math.Floor(float64(curW)*scale2)))
	curH = max(1, int(math.Floor(float64(curH)*scale2)))
	return curW, curH
}

func prepareVisionAttachment(ctx context.Context, file *os.File, info os.FileInfo, img image.Image, format string, mimeType string) *tool.ImageAttachment {
	if err := ctx.Err(); err != nil {
		return nil
	}
	if info.Size() > maxVisionAttachBytes {
		return nil
	}
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil
	}

	targetW, targetH := visionTargetDimensions(srcW, srcH, defaultMaxVisionDimension, defaultMaxVisionPatches)

	// If source fits patch budget and format is pass-through safe, use raw bytes.
	if targetW == srcW && targetH == srcH && (format == "png" || format == "jpeg" || format == "webp") {
		if _, err := file.Seek(0, io.SeekStart); err == nil {
			rawBytes := make([]byte, info.Size())
			if _, err := io.ReadFull(file, rawBytes); err == nil {
				return &tool.ImageAttachment{
					MIMEType: mimeType,
					Data:     base64.StdEncoding.EncodeToString(rawBytes),
					Width:    srcW,
					Height:   srcH,
				}
			}
		}
	}

	// Downsample to target dimensions.
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	var buf bytes.Buffer
	var outMIME string
	if format == "jpeg" {
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
			return nil
		}
		outMIME = "image/jpeg"
	} else {
		if err := png.Encode(&buf, dst); err != nil {
			return nil
		}
		outMIME = "image/png"
	}

	if buf.Len() > maxVisionAttachBytes {
		return nil
	}

	return &tool.ImageAttachment{
		MIMEType: outMIME,
		Data:     base64.StdEncoding.EncodeToString(buf.Bytes()),
		Width:    targetW,
		Height:   targetH,
	}
}
