package imageprep

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp"
)

const MaxSnapshotBytes = 32 * 1024 * 1024

type Snapshot struct {
	MIMEType string
	Data     string
	Width    int
	Height   int
}

// ValidateSourceDimensions applies the same allocation-safety bound used before
// full image decode. Input adapters can reject unsafe images before enqueueing.
func ValidateSourceDimensions(width, height int) error {
	return validateSourceDimensions(width, height)
}

func SnapshotLocal(path string) (Snapshot, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Snapshot{}, fmt.Errorf("image path is empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open image %q: %w", filepath.Base(path), err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Snapshot{}, fmt.Errorf("stat image %q: %w", filepath.Base(path), err)
	}
	if !info.Mode().IsRegular() {
		return Snapshot{}, fmt.Errorf("image %q is not a regular file", filepath.Base(path))
	}
	if info.Size() > MaxSnapshotBytes {
		return Snapshot{}, fmt.Errorf("image %q exceeds %d byte limit", filepath.Base(path), MaxSnapshotBytes)
	}

	data, err := io.ReadAll(io.LimitReader(file, MaxSnapshotBytes+1))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read image %q: %w", filepath.Base(path), err)
	}
	if len(data) > MaxSnapshotBytes {
		return Snapshot{}, fmt.Errorf("image %q exceeds %d byte limit", filepath.Base(path), MaxSnapshotBytes)
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Snapshot{}, fmt.Errorf("decode image %q: %w", filepath.Base(path), err)
	}
	mime, ok := mimeForFormat(format)
	if !ok {
		return Snapshot{}, fmt.Errorf("unsupported image format %q", format)
	}
	if err := ValidateSourceDimensions(config.Width, config.Height); err != nil {
		return Snapshot{}, fmt.Errorf("image %q: %w", filepath.Base(path), err)
	}

	return Snapshot{
		MIMEType: mime,
		Data:     base64.StdEncoding.EncodeToString(data),
		Width:    config.Width,
		Height:   config.Height,
	}, nil
}

func mimeForFormat(format string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return "image/png", true
	case "jpeg":
		return "image/jpeg", true
	case "gif":
		return "image/gif", true
	case "webp":
		return "image/webp", true
	default:
		return "", false
	}
}
