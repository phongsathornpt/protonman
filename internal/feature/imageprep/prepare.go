package imageprep

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	xdraw "golang.org/x/image/draw"
)

const (
	PatchSize             = 32
	DefaultMaxDimension   = 2048
	DefaultMaxPatches     = 2500
	OriginalMaxDimension  = 6000
	OriginalMaxPatches    = 10000
	DefaultMaxOutputBytes = 10 * 1024 * 1024
	// MaxSourcePixels bounds full image decode allocations. Metadata is checked
	// against this limit before image.Decode can allocate a pixel buffer.
	MaxSourcePixels = 12 * 1024 * 1024
	// MaxSourceDimension also rejects pathological skinny images before decode.
	MaxSourceDimension = 16384
	maxCacheEntries    = 32
	maxCacheBytes      = 64 * 1024 * 1024
)

// Policy describes the model-visible image budget. It intentionally models
// dimensions/patches rather than encoded transport bytes.
type Policy struct {
	MaxDimension   int
	MaxPatches     int
	PatchSize      int
	MaxOutputBytes int
}

func DefaultPolicy() Policy {
	return Policy{
		MaxDimension:   DefaultMaxDimension,
		MaxPatches:     DefaultMaxPatches,
		PatchSize:      PatchSize,
		MaxOutputBytes: DefaultMaxOutputBytes,
	}
}

func OriginalPolicy() Policy {
	return Policy{
		MaxDimension:   OriginalMaxDimension,
		MaxPatches:     OriginalMaxPatches,
		PatchSize:      PatchSize,
		MaxOutputBytes: DefaultMaxOutputBytes,
	}
}

func (p Policy) normalized() Policy {
	if p.MaxDimension <= 0 {
		p.MaxDimension = DefaultMaxDimension
	}
	if p.MaxPatches <= 0 {
		p.MaxPatches = DefaultMaxPatches
	}
	if p.PatchSize <= 0 {
		p.PatchSize = PatchSize
	}
	if p.MaxOutputBytes <= 0 {
		p.MaxOutputBytes = DefaultMaxOutputBytes
	}
	return p
}

type Prepared struct {
	MIMEType     string
	Data         string
	SourceWidth  int
	SourceHeight int
	Width        int
	Height       int
}

func (p Prepared) Resized() bool {
	return p.SourceWidth != p.Width || p.SourceHeight != p.Height
}

type prepareCacheKey struct {
	Digest [32]byte
	Policy Policy
}

type prepareCacheEntry struct {
	key   prepareCacheKey
	value Prepared
	size  int
}

type imagePrepareCache struct {
	mu    sync.Mutex
	items map[prepareCacheKey]*list.Element
	lru   list.List
	bytes int
}

var promptImageCache = imagePrepareCache{items: make(map[prepareCacheKey]*list.Element)}

func (c *imagePrepareCache) get(key prepareCacheKey) (Prepared, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.items[key]
	if !ok {
		return Prepared{}, false
	}
	c.lru.MoveToFront(element)
	return element.Value.(prepareCacheEntry).value, true
}

func (c *imagePrepareCache) put(key prepareCacheKey, value Prepared) {
	size := len(value.Data)
	if size <= 0 || size > maxCacheBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.items[key]; ok {
		entry := existing.Value.(prepareCacheEntry)
		c.bytes -= entry.size
		entry.value = value
		entry.size = size
		existing.Value = entry
		c.bytes += size
		c.lru.MoveToFront(existing)
	} else {
		entry := prepareCacheEntry{key: key, value: value, size: size}
		element := c.lru.PushFront(entry)
		c.items[key] = element
		c.bytes += size
	}
	for c.lru.Len() > maxCacheEntries || c.bytes > maxCacheBytes {
		last := c.lru.Back()
		if last == nil {
			break
		}
		entry := last.Value.(prepareCacheEntry)
		delete(c.items, entry.key)
		c.bytes -= entry.size
		c.lru.Remove(last)
	}
}

// PrepareMessages clones messages and prepares every image part for a model
// request. Conversation history retains the canonical source-quality snapshot.
func PrepareMessages(messages []domain.Message, policy Policy) ([]domain.Message, error) {
	prepared := domain.CloneMessages(messages)
	for messageIndex := range prepared {
		for partIndex := range prepared[messageIndex].Parts {
			part := &prepared[messageIndex].Parts[partIndex]
			if part.Type != domain.ContentPartImage {
				continue
			}
			image, err := Prepare(part.MIMEType, part.Data, policy)
			if err != nil {
				return nil, fmt.Errorf("prepare image part in message %d: %w", messageIndex, err)
			}
			part.MIMEType = image.MIMEType
			part.Data = image.Data
		}
	}
	return prepared, nil
}

func Prepare(mimeType, data string, policy Policy) (Prepared, error) {
	policy = policy.normalized()
	data = strings.TrimSpace(data)
	if data == "" {
		return Prepared{}, fmt.Errorf("image data is empty")
	}
	if len(data) > base64.StdEncoding.EncodedLen(MaxSnapshotBytes) {
		return Prepared{}, fmt.Errorf("image payload exceeds %d byte source limit", MaxSnapshotBytes)
	}
	key := prepareCacheKey{Digest: sha256.Sum256([]byte(data)), Policy: policy}
	if cached, ok := promptImageCache.get(key); ok {
		return cached, nil
	}

	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return Prepared{}, fmt.Errorf("decode image base64: %w", err)
	}
	if len(raw) > MaxSnapshotBytes {
		return Prepared{}, fmt.Errorf("image payload exceeds %d byte source limit", MaxSnapshotBytes)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return Prepared{}, fmt.Errorf("decode image metadata: %w", err)
	}
	if err := validateSourceDimensions(config.Width, config.Height); err != nil {
		return Prepared{}, err
	}

	// Full decode validates the complete image, but the config check above runs
	// first so malformed compressed input cannot request an unbounded allocation.
	img, decodedFormat, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return Prepared{}, fmt.Errorf("decode image: %w", err)
	}
	if decodedFormat != "" {
		format = decodedFormat
	}
	bounds := img.Bounds()
	sourceWidth, sourceHeight := bounds.Dx(), bounds.Dy()
	if err := validateSourceDimensions(sourceWidth, sourceHeight); err != nil {
		return Prepared{}, err
	}
	targetWidth, targetHeight := OutputDimensions(sourceWidth, sourceHeight, policy)

	// Preserve source encoding and pixels when only validation was needed.
	if targetWidth == sourceWidth && targetHeight == sourceHeight &&
		canPreserveSource(format) && len(raw) <= policy.MaxOutputBytes {
		prepared := Prepared{
			MIMEType:     normalizedMIME(format, mimeType),
			Data:         data,
			SourceWidth:  sourceWidth,
			SourceHeight: sourceHeight,
			Width:        sourceWidth,
			Height:       sourceHeight,
		}
		promptImageCache.put(key, prepared)
		return prepared, nil
	}

	working := img
	if targetWidth != sourceWidth || targetHeight != sourceHeight {
		dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
		xdraw.BiLinear.Scale(dst, dst.Bounds(), img, bounds, xdraw.Over, nil)
		working = dst
	}

	encoded, outputMIME, err := encodePreparedImage(working, format, policy.MaxOutputBytes)
	if err != nil {
		return Prepared{}, err
	}
	prepared := Prepared{
		MIMEType:     outputMIME,
		Data:         base64.StdEncoding.EncodeToString(encoded),
		SourceWidth:  sourceWidth,
		SourceHeight: sourceHeight,
		Width:        targetWidth,
		Height:       targetHeight,
	}
	promptImageCache.put(key, prepared)
	return prepared, nil
}

func validateSourceDimensions(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid image dimensions %dx%d", width, height)
	}
	if width > MaxSourceDimension || height > MaxSourceDimension || int64(width)*int64(height) > int64(MaxSourcePixels) {
		return fmt.Errorf("image dimensions %dx%d exceed safe decode limit", width, height)
	}
	return nil
}

// OutputDimensions follows Codex's prompt image policy: cap the longest side,
// then cap the 32px patch grid by area while preserving aspect ratio.
func OutputDimensions(width, height int, policy Policy) (int, int) {
	policy = policy.normalized()
	width = max(1, width)
	height = max(1, height)
	if dimensionsFit(width, height, policy) {
		return width, height
	}

	maxSide := max(width, height)
	scale := math.Min(1, float64(policy.MaxDimension)/float64(maxSide))
	width = max(1, int(math.Round(float64(width)*scale)))
	height = max(1, int(math.Round(float64(height)*scale)))
	if dimensionsFit(width, height, policy) {
		return width, height
	}

	widthF := float64(width)
	heightF := float64(height)
	patch := float64(policy.PatchSize)
	scale = math.Sqrt(patch * patch * float64(policy.MaxPatches) / widthF / heightF)
	scaledWide := widthF * scale / patch
	scaledHigh := heightF * scale / patch
	if scaledWide > 0 && scaledHigh > 0 {
		wideFloor := math.Max(1, math.Floor(scaledWide))
		highFloor := math.Max(1, math.Floor(scaledHigh))
		scale *= math.Min(wideFloor/scaledWide, highFloor/scaledHigh)
	}
	return max(1, int(math.Floor(widthF*scale))), max(1, int(math.Floor(heightF*scale)))
}

func dimensionsFit(width, height int, policy Policy) bool {
	if width > policy.MaxDimension || height > policy.MaxDimension {
		return false
	}
	patchesWide := (width + policy.PatchSize - 1) / policy.PatchSize
	patchesHigh := (height + policy.PatchSize - 1) / policy.PatchSize
	return int64(patchesWide)*int64(patchesHigh) <= int64(policy.MaxPatches)
}

func canPreserveSource(format string) bool {
	switch strings.ToLower(format) {
	case "png", "jpeg", "webp":
		return true
	default:
		return false
	}
}

func normalizedMIME(format, supplied string) string {
	switch strings.ToLower(format) {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	default:
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(supplied)), "image/") {
			return strings.TrimSpace(supplied)
		}
		return "image/png"
	}
}

func encodePreparedImage(img image.Image, sourceFormat string, maxBytes int) ([]byte, string, error) {
	var buffer bytes.Buffer
	if strings.EqualFold(sourceFormat, "jpeg") && opaque(img) {
		if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", fmt.Errorf("encode jpeg: %w", err)
		}
		if buffer.Len() <= maxBytes {
			return buffer.Bytes(), "image/jpeg", nil
		}
		buffer.Reset()
	}

	if err := png.Encode(&buffer, img); err != nil {
		return nil, "", fmt.Errorf("encode png: %w", err)
	}
	if buffer.Len() <= maxBytes {
		return buffer.Bytes(), "image/png", nil
	}
	if opaque(img) {
		buffer.Reset()
		if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", fmt.Errorf("encode jpeg fallback: %w", err)
		}
		if buffer.Len() <= maxBytes {
			return buffer.Bytes(), "image/jpeg", nil
		}
	}
	return nil, "", fmt.Errorf("prepared image exceeds %d byte limit", maxBytes)
}

func opaque(img image.Image) bool {
	if candidate, ok := img.(interface{ Opaque() bool }); ok {
		return candidate.Opaque()
	}
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			if alpha != 0xffff {
				return false
			}
		}
	}
	return true
}
