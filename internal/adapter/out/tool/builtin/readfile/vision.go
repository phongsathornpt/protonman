package readfile

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"golang.org/x/image/draw"
)

const (
	maxVisionDimension   = 1568
	maxVisionPixels      = 1_600_000
	maxVisionAttachBytes = 10 * 1024 * 1024 // 10 MiB limit for base64 attachment
	maxRawPassSize       = 1024 * 1024      // 1 MiB limit for raw pass-through
)

func visionTargetDimensions(width, height int) (int, int) {
	if width <= 0 || height <= 0 {
		return 1, 1
	}
	if width <= maxVisionDimension && height <= maxVisionDimension && int64(width)*int64(height) <= int64(maxVisionPixels) {
		return width, height
	}
	scale := 1.0
	maxSide := max(width, height)
	if maxSide > maxVisionDimension {
		scale = float64(maxVisionDimension) / float64(maxSide)
	}
	curW := max(1, int(math.Round(float64(width)*scale)))
	curH := max(1, int(math.Round(float64(height)*scale)))
	if int64(curW)*int64(curH) <= int64(maxVisionPixels) {
		return curW, curH
	}
	scale2 := math.Sqrt(float64(maxVisionPixels) / float64(curW*curH))
	curW = max(1, int(math.Floor(float64(curW)*scale2)))
	curH = max(1, int(math.Floor(float64(curH)*scale2)))
	return curW, curH
}

// VisionTokenEstimate contains estimated token cost for major multimodal vision models.
type VisionTokenEstimate struct {
	OpenAI    int `json:"openai"`
	Anthropic int `json:"anthropic"`
}

func estimateVisionTokens(width, height int) VisionTokenEstimate {
	if width <= 0 || height <= 0 {
		return VisionTokenEstimate{}
	}
	// OpenAI high detail: 85 base + 170 per 512x512 tile
	tilesX := int(math.Ceil(float64(width) / 512.0))
	tilesY := int(math.Ceil(float64(height) / 512.0))
	openAITokens := (tilesX * tilesY * 170) + 85

	// Anthropic: ~750 pixels per token (minimum 1)
	anthropicTokens := max(1, int(math.Round(float64(width*height)/750.0)))

	return VisionTokenEstimate{
		OpenAI:    openAITokens,
		Anthropic: anthropicTokens,
	}
}

func flattenToOpaque(src image.Image, bounds image.Rectangle) *image.RGBA {
	bg := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(bg, bg.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(bg, bg.Bounds(), src, bounds.Min, draw.Over)
	return bg
}

func prepareVisionAttachment(ctx context.Context, file *os.File, info os.FileInfo, img image.Image, format string, mimeType string) (*tool.ImageAttachment, VisionTokenEstimate) {
	if err := ctx.Err(); err != nil {
		return nil, VisionTokenEstimate{}
	}
	if info.Size() > maxVisionAttachBytes {
		return nil, VisionTokenEstimate{}
	}
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, VisionTokenEstimate{}
	}

	targetW, targetH := visionTargetDimensions(srcW, srcH)
	estimate := estimateVisionTokens(targetW, targetH)

	// If source fits target budget, is already compact, and format is pass-through safe, use raw bytes.
	if targetW == srcW && targetH == srcH && info.Size() <= maxRawPassSize && (format == "png" || format == "jpeg" || format == "webp") {
		if _, err := file.Seek(0, io.SeekStart); err == nil {
			rawBytes := make([]byte, info.Size())
			if _, err := io.ReadFull(file, rawBytes); err == nil {
				return &tool.ImageAttachment{
					MIMEType: mimeType,
					Data:     base64.StdEncoding.EncodeToString(rawBytes),
					Width:    srcW,
					Height:   srcH,
				}, estimate
			}
		}
	}

	// Downsample to target dimensions using pure Go Catmull-Rom for sharp edges and text clarity.
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	isOpaque, _ := imageOpaque(ctx, dst)
	var buf bytes.Buffer
	var outMIME string

	if isOpaque || format == "jpeg" {
		toEncode := dst
		if !isOpaque {
			toEncode = flattenToOpaque(dst, dst.Bounds())
		}
		if err := jpeg.Encode(&buf, toEncode, &jpeg.Options{Quality: 85}); err != nil {
			return nil, estimate
		}
		outMIME = "image/jpeg"
	} else {
		if err := png.Encode(&buf, dst); err != nil {
			return nil, estimate
		}
		outMIME = "image/png"
		// If PNG is bloated (> 2 MiB), flatten over white canvas and re-encode as JPEG Quality 85.
		if buf.Len() > 2*1024*1024 {
			buf.Reset()
			flattened := flattenToOpaque(dst, dst.Bounds())
			if err := jpeg.Encode(&buf, flattened, &jpeg.Options{Quality: 85}); err != nil {
				return nil, estimate
			}
			outMIME = "image/jpeg"
		}
	}

	if buf.Len() > maxVisionAttachBytes {
		return nil, estimate
	}

	return &tool.ImageAttachment{
		MIMEType: outMIME,
		Data:     base64.StdEncoding.EncodeToString(buf.Bytes()),
		Width:    targetW,
		Height:   targetH,
	}, estimate
}
