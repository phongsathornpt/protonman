package readfile

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	baseanalysis "github.com/phongsathornpt/protonman/internal/base/analysis"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	_ "golang.org/x/image/webp"
)

const (
	maxImagePixels       = 12 * 1024 * 1024
	maxImageSamples      = 64 * 1024
	maxEncodedImageBytes = 32 * 1024 * 1024
	maxImageColors       = 8
	imageRegionCols      = 4
	imageRegionRows      = 4
	imageASCIIWidth      = 48
	imageASCIIHeight     = 16
)

type imageMetadata struct {
	Format        string `json:"format"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Opaque        bool   `json:"opaque"`
	AnalysisScope string `json:"analysis_scope,omitempty"`
}

type dominantColor struct {
	Hex   string   `json:"hex"`
	RGB   [3]uint8 `json:"rgb"`
	Count int      `json:"count"`
}

type imageRegion struct {
	Row        int                  `json:"row"`
	Column     int                  `json:"column"`
	Brightness baseanalysis.Summary `json:"brightness"`
}

type imageAnalysis struct {
	Samples        int                  `json:"samples"`
	Brightness     baseanalysis.Summary `json:"brightness"`
	DominantColors []dominantColor      `json:"dominant_colors"`
	Regions        []imageRegion        `json:"regions,omitempty"`
	EdgeDensity    float64              `json:"edge_density"`
	ASCIIPreview   string               `json:"ascii_preview,omitempty"`
}

type colorBucket struct {
	key   int
	count int
}

func readImageArtifact(ctx context.Context, file *os.File, info os.FileInfo, input readFileInput, artifact artifactInfo, call tool.Call) (tool.Result, error) {
	defer file.Close()
	if info.Size() > maxEncodedImageBytes {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, fmt.Sprintf("read image size %d bytes exceeds the safe decode limit", info.Size()))
	}
	config, format, err := image.DecodeConfig(&contextReader{ctx: ctx, reader: file})
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode image metadata", err)
	}
	if !imageDimensionsWithinLimit(config.Width, config.Height) {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, fmt.Sprintf("read image dimensions %dx%d exceed the safe analysis limit", config.Width, config.Height))
	}
	if _, err := file.Seek(0, 0); err != nil {
		return tool.Result{}, fmt.Errorf("rewind %q: %w", input.Path, err)
	}
	img, _, err := image.Decode(&contextReader{ctx: ctx, reader: file})
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode image", err)
	}
	bounds := img.Bounds()
	cols, rows := imageSampleGrid(bounds.Dx(), bounds.Dy(), maxImageSamples)

	var histogram [4096]int
	samples := 0
	var brightnessStats baseanalysis.RunningStats
	regionStats := make([]baseanalysis.RunningStats, imageRegionRows*imageRegionCols)
	edgeComparisons, edgeHits := 0, 0
	var previousRow []float64
	for row := 0; row < rows; row++ {
		if err := ctx.Err(); err != nil {
			return tool.Result{}, fmt.Errorf("analyze image %q: %w", input.Path, err)
		}
		currentRow := make([]float64, 0, cols)
		for col := 0; col < cols; col++ {
			x := bounds.Min.X + stratifiedCoordinate(bounds.Dx(), cols, col, row, 0x9e3779b97f4a7c15)
			y := bounds.Min.Y + stratifiedCoordinate(bounds.Dy(), rows, row, col, 0xbf58476d1ce4e5b9)
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			samples++
			if a16 == 0 {
				currentRow = append(currentRow, math.NaN())
				continue
			}
			r, g, b := unpremultiplyRGBA(r16, g16, b16, a16)
			brightness := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			brightnessStats.Add(brightness)
			regionCol := col * imageRegionCols / cols
			regionRow := row * imageRegionRows / rows
			regionStats[regionRow*imageRegionCols+regionCol].Add(brightness)
			index := len(currentRow)
			if index > 0 && !math.IsNaN(currentRow[index-1]) {
				edgeComparisons++
				if math.Abs(brightness-currentRow[index-1]) >= 24 {
					edgeHits++
				}
			}
			if index < len(previousRow) && !math.IsNaN(previousRow[index]) {
				edgeComparisons++
				if math.Abs(brightness-previousRow[index]) >= 24 {
					edgeHits++
				}
			}
			currentRow = append(currentRow, brightness)
			key := int(r>>4)<<8 | int(g>>4)<<4 | int(b>>4)
			histogram[key]++
		}
		previousRow = currentRow
	}
	buckets := make([]colorBucket, 0, len(histogram))
	for key, count := range histogram {
		if count > 0 {
			buckets = append(buckets, colorBucket{key: key, count: count})
		}
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].count == buckets[j].count {
			return buckets[i].key < buckets[j].key
		}
		return buckets[i].count > buckets[j].count
	})
	if len(buckets) > maxImageColors {
		buckets = buckets[:maxImageColors]
	}
	colors := make([]dominantColor, 0, len(buckets))
	for _, bucket := range buckets {
		r := uint8(((bucket.key >> 8) & 0xf) << 4)
		g := uint8(((bucket.key >> 4) & 0xf) << 4)
		b := uint8((bucket.key & 0xf) << 4)
		r, g, b = r+8, g+8, b+8
		colors = append(colors, dominantColor{Hex: fmt.Sprintf("#%02x%02x%02x", r, g, b), RGB: [3]uint8{r, g, b}, Count: bucket.count})
	}
	opaque, err := imageOpaque(ctx, img)
	if err != nil {
		return tool.Result{}, fmt.Errorf("inspect image opacity %q: %w", input.Path, err)
	}
	summary := brightnessStats.Summary()
	regions := make([]imageRegion, 0, len(regionStats))
	for index, stats := range regionStats {
		regions = append(regions, imageRegion{Row: index / imageRegionCols, Column: index % imageRegionCols, Brightness: stats.Summary()})
	}
	edgeDensity := 0.0
	if edgeComparisons > 0 {
		edgeDensity = float64(edgeHits) / float64(edgeComparisons)
	}
	analysis := imageAnalysis{Samples: samples, Brightness: summary, DominantColors: colors, Regions: regions, EdgeDensity: edgeDensity, ASCIIPreview: imageASCIIPreview(img, imageASCIIWidth, imageASCIIHeight)}
	metadata := imageMetadata{Format: format, Width: config.Width, Height: config.Height, Opaque: opaque}
	if format == "gif" {
		metadata.AnalysisScope = "first_frame"
	}
	output := fmt.Sprintf("image %s %dx%d · sampled %d px · brightness mean %.1f min %.1f max %.1f", format, config.Width, config.Height, samples, summary.Mean, summary.Min, summary.Max)
	if len(colors) > 0 {
		output += fmt.Sprintf(" · dominant %s", colors[0].Hex)
	}
	return artifactResult(call, artifactEnvelope{Kind: artifactImage, Path: input.Path, MIMEType: artifact.MIMEType, SizeBytes: info.Size(), Metadata: metadata, Analysis: analysis}, output)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func imageSampleGrid(width, height, budget int) (int, int) {
	if width <= 0 || height <= 0 || budget <= 0 {
		return 0, 0
	}
	if int64(width)*int64(height) <= int64(budget) {
		return width, height
	}
	aspect := float64(width) / float64(height)
	cols := int(math.Sqrt(float64(budget) * aspect))
	if cols < 1 {
		cols = 1
	}
	if cols > width {
		cols = width
	}
	if cols > budget {
		cols = budget
	}
	rows := budget / cols
	if rows < 1 {
		rows = 1
	}
	if rows > height {
		rows = height
		cols = budget / rows
		if cols > width {
			cols = width
		}
	}
	return cols, rows
}

func stratifiedCoordinate(size, cells, index, phase int, salt uint64) int {
	start := index * size / cells
	end := (index + 1) * size / cells
	span := end - start
	if span <= 1 {
		return start
	}
	h := uint64(index+1)*0x9e3779b97f4a7c15 ^ uint64(phase+1)*0xbf58476d1ce4e5b9 ^ salt
	h ^= h >> 30
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 27
	return start + int(h%uint64(span))
}

func unpremultiplyRGBA(r16, g16, b16, a16 uint32) (uint8, uint8, uint8) {
	if a16 == 0 {
		return 0, 0, 0
	}
	if a16 == 0xffff {
		return uint8(r16 >> 8), uint8(g16 >> 8), uint8(b16 >> 8)
	}
	return uint8(min(uint32(0xffff), r16*0xffff/a16) >> 8), uint8(min(uint32(0xffff), g16*0xffff/a16) >> 8), uint8(min(uint32(0xffff), b16*0xffff/a16) >> 8)
}

func imageOpaque(ctx context.Context, img image.Image) (bool, error) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0xffff {
				return false, nil
			}
		}
	}
	return true, nil
}

func imageDimensionsWithinLimit(width, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	return int64(width)*int64(height) <= int64(maxImagePixels)
}

func imageASCIIPreview(img image.Image, width, height int) string {
	bounds := img.Bounds()
	if width <= 0 || height <= 0 || bounds.Empty() {
		return ""
	}
	if width > bounds.Dx() {
		width = bounds.Dx()
	}
	if height > bounds.Dy() {
		height = bounds.Dy()
	}
	const ramp = " .:-=+*#%@"
	var out strings.Builder
	out.Grow((width + 1) * height)
	for row := 0; row < height; row++ {
		y := bounds.Min.Y + row*bounds.Dy()/height
		for col := 0; col < width; col++ {
			x := bounds.Min.X + col*bounds.Dx()/width
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			if a16 == 0 {
				out.WriteByte(' ')
				continue
			}
			r, g, b := unpremultiplyRGBA(r16, g16, b16, a16)
			brightness := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			index := int(brightness * float64(len(ramp)-1) / 255.0)
			out.WriteByte(ramp[index])
		}
		if row+1 < height {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
