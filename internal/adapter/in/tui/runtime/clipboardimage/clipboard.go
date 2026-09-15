//go:build darwin || linux

package clipboardimage

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png"
	"os"

	"github.com/phongsathornpt/protonman/internal/feature/imageprep"
	"golang.design/x/clipboard"
)

type Result struct {
	Path   string
	Width  int
	Height int
	Err    error
}

func Load() Result {
	var result Result
	if err := clipboard.Init(); err != nil {
		result.Err = fmt.Errorf("clipboard unavailable: %w", err)
		return result
	}
	raw := clipboard.Read(clipboard.FmtImage)
	if len(raw) == 0 {
		result.Err = fmt.Errorf("clipboard does not contain an image")
		return result
	}
	if len(raw) > imageprep.MaxSnapshotBytes {
		result.Err = fmt.Errorf("clipboard image exceeds %d byte limit", imageprep.MaxSnapshotBytes)
		return result
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		result.Err = fmt.Errorf("decode clipboard image: %w", err)
		return result
	}
	if format != "png" {
		result.Err = fmt.Errorf("clipboard image format %q is not PNG", format)
		return result
	}
	if err := imageprep.ValidateSourceDimensions(config.Width, config.Height); err != nil {
		result.Err = fmt.Errorf("clipboard image: %w", err)
		return result
	}
	path, err := WriteTempPNG(raw)
	if err != nil {
		result.Err = err
		return result
	}
	result.Path = path
	result.Width = config.Width
	result.Height = config.Height
	return result
}

func WriteTempPNG(raw []byte) (path string, writeErr error) {
	file, err := os.CreateTemp("", "protonman-clipboard-*.png")
	if err != nil {
		return "", fmt.Errorf("create clipboard image temp file: %w", err)
	}
	path = file.Name()
	closed := false
	defer func() {
		if !closed {
			if err := file.Close(); err != nil && writeErr == nil {
				writeErr = fmt.Errorf("close clipboard image temp file: %w", err)
			}
		}
		if writeErr != nil {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", fmt.Errorf("protect clipboard image temp file: %w", err)
	}
	if _, err := file.Write(raw); err != nil {
		return "", fmt.Errorf("write clipboard image temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close clipboard image temp file: %w", err)
	}
	closed = true
	return path, nil
}
