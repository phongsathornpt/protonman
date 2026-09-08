package readfile

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"sort"
	"strings"

	baseanalysis "github.com/phongsathornpt/protonman/internal/base/analysis"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

const (
	maxImagePixels   = 32 * 1024 * 1024
	maxImageSamples  = 64 * 1024
	maxImageColors   = 8
	imageRegionCols  = 4
	imageRegionRows  = 4
	imageASCIIWidth  = 48
	imageASCIIHeight = 16
)

type imageMetadata struct {
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Opaque bool   `json:"opaque"`
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
	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode image metadata", err)
	}
	pixels := int64(config.Width) * int64(config.Height)
	if config.Width <= 0 || config.Height <= 0 || pixels > maxImagePixels {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, fmt.Sprintf("read image dimensions %dx%d exceed the safe analysis limit", config.Width, config.Height))
	}
	if _, err := file.Seek(0, 0); err != nil {
		return tool.Result{}, fmt.Errorf("rewind %q: %w", input.Path, err)
	}
	img, _, err := image.Decode(file)
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode image", err)
	}
	bounds := img.Bounds()
	step := 1
	if pixels > maxImageSamples {
		step = int(math.Ceil(math.Sqrt(float64(pixels) / float64(maxImageSamples))))
	}

	var histogram [4096]int
	samples := 0
	opaque := true
	var brightnessStats baseanalysis.RunningStats
	regionStats := make([]baseanalysis.RunningStats, imageRegionRows*imageRegionCols)
	edgeComparisons, edgeHits := 0, 0
	var previousRow []float64
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		if err := ctx.Err(); err != nil {
			return tool.Result{}, fmt.Errorf("analyze image %q: %w", input.Path, err)
		}
		currentRow := make([]float64, 0, (bounds.Dx()+step-1)/step)
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			r, g, b := uint8(r16>>8), uint8(g16>>8), uint8(b16>>8)
			if a16 != 0xffff {
				opaque = false
			}
			brightness := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			samples++
			brightnessStats.Add(brightness)
			regionCol := (x - bounds.Min.X) * imageRegionCols / bounds.Dx()
			regionRow := (y - bounds.Min.Y) * imageRegionRows / bounds.Dy()
			if regionCol >= imageRegionCols {
				regionCol = imageRegionCols - 1
			}
			if regionRow >= imageRegionRows {
				regionRow = imageRegionRows - 1
			}
			regionStats[regionRow*imageRegionCols+regionCol].Add(brightness)
			index := len(currentRow)
			if index > 0 {
				edgeComparisons++
				if math.Abs(brightness-currentRow[index-1]) >= 24 {
					edgeHits++
				}
			}
			if index < len(previousRow) {
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
		colors = append(colors, dominantColor{
			Hex: fmt.Sprintf("#%02x%02x%02x", r, g, b),
			RGB: [3]uint8{r, g, b}, Count: bucket.count,
		})
	}
	summary := brightnessStats.Summary()
	regions := make([]imageRegion, 0, len(regionStats))
	for index, stats := range regionStats {
		regions = append(regions, imageRegion{
			Row: index / imageRegionCols, Column: index % imageRegionCols, Brightness: stats.Summary(),
		})
	}
	edgeDensity := 0.0
	if edgeComparisons > 0 {
		edgeDensity = float64(edgeHits) / float64(edgeComparisons)
	}
	analysis := imageAnalysis{
		Samples: samples, Brightness: summary, DominantColors: colors, Regions: regions,
		EdgeDensity: edgeDensity, ASCIIPreview: imageASCIIPreview(img, imageASCIIWidth, imageASCIIHeight),
	}
	metadata := imageMetadata{Format: format, Width: config.Width, Height: config.Height, Opaque: opaque}
	output := fmt.Sprintf("image %s %dx%d · sampled %d px · brightness mean %.1f min %.1f max %.1f", format, config.Width, config.Height, samples, summary.Mean, summary.Min, summary.Max)
	if len(colors) > 0 {
		output += fmt.Sprintf(" · dominant %s", colors[0].Hex)
	}
	return artifactResult(call, artifactEnvelope{
		Kind: artifactImage, Path: input.Path, MIMEType: artifact.MIMEType,
		SizeBytes: info.Size(), Metadata: metadata, Analysis: analysis,
	}, output)
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
			r16, g16, b16, _ := img.At(x, y).RGBA()
			brightness := 0.2126*float64(r16>>8) + 0.7152*float64(g16>>8) + 0.0722*float64(b16>>8)
			index := int(brightness * float64(len(ramp)-1) / 255.0)
			out.WriteByte(ramp[index])
		}
		if row+1 < height {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
