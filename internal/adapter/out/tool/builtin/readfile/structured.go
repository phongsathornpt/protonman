package readfile

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	baseanalysis "github.com/phongsathornpt/protonman/internal/base/analysis"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

const (
	maxStructuredPreviewRows = 5
	maxStructuredFields      = 64
)

type structuredMetadata struct {
	Format   string   `json:"format"`
	TopLevel string   `json:"top_level,omitempty"`
	Rows     int      `json:"rows,omitempty"`
	Columns  int      `json:"columns,omitempty"`
	Fields   []string `json:"fields,omitempty"`
}

type numericSummary = baseanalysis.Summary
type structuredAnalysis struct {
	Preview    any                       `json:"preview,omitempty"`
	Statistics map[string]numericSummary `json:"statistics,omitempty"`
}

type numericAccumulator struct {
	stats baseanalysis.RunningStats
}

func (a *numericAccumulator) add(value float64) { a.stats.Add(value) }

func (a numericAccumulator) summary() numericSummary { return a.stats.Summary() }
func readStructuredArtifact(ctx context.Context, file *os.File, info os.FileInfo, input readFileInput, artifact artifactInfo, call tool.Call) (tool.Result, error) {
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(MaxReadFileBytes)+1))
	if err != nil {
		return tool.Result{}, fmt.Errorf("read structured file %q: %w", input.Path, err)
	}
	truncated := len(data) > MaxReadFileBytes
	if truncated {
		data = data[:MaxReadFileBytes]
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{}, fmt.Errorf("analyze structured file %q: %w", input.Path, err)
	}

	var metadata structuredMetadata
	var analysis structuredAnalysis
	switch artifact.Kind {
	case artifactJSON:
		if truncated {
			return structuredLimitResult(call, input, artifact, info)
		}
		metadata, analysis, err = analyzeJSON(data)
	case artifactJSONL:
		metadata, analysis, err = analyzeJSONL(data, truncated)
	case artifactCSV, artifactTSV:
		metadata, analysis, err = analyzeDelimited(data, artifact.Kind, truncated)
	default:
		err = fmt.Errorf("unsupported structured artifact kind %q", artifact.Kind)
	}
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "analyze structured file", err)
	}
	output := fmt.Sprintf("structured %s · rows %d", metadata.Format, metadata.Rows)
	if metadata.Columns > 0 {
		output += fmt.Sprintf(" · columns %d", metadata.Columns)
	}
	if truncated {
		output += " · bounded preview"
	}
	return artifactResult(call, artifactEnvelope{
		Kind: artifact.Kind, Path: input.Path, MIMEType: artifact.MIMEType,
		SizeBytes: info.Size(), Metadata: metadata, Analysis: analysis, Truncated: truncated,
	}, output)
}

func structuredLimitResult(call tool.Call, input readFileInput, artifact artifactInfo, info os.FileInfo) (tool.Result, error) {
	return artifactResult(call, artifactEnvelope{
		Kind: artifact.Kind, Path: input.Path, MIMEType: artifact.MIMEType,
		SizeBytes: info.Size(), Truncated: true,
		Description: "structured JSON analysis is bounded to 2 MiB; use text byte pagination for this file",
	}, fmt.Sprintf("structured json · %d bytes · analysis bounded at 2 MiB; use text pagination", info.Size()))
}

func analyzeJSON(data []byte) (structuredMetadata, structuredAnalysis, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return structuredMetadata{}, structuredAnalysis{}, err
	}
	return summarizeJSONValue(value)
}
func summarizeJSONValue(value any) (structuredMetadata, structuredAnalysis, error) {
	metadata := structuredMetadata{Format: "json"}
	analysis := structuredAnalysis{}
	switch typed := value.(type) {
	case []any:
		metadata.TopLevel = "array"
		metadata.Rows = len(typed)
		metadata.Fields = collectObjectFields(typed)
		metadata.Columns = len(metadata.Fields)
		analysis.Preview = previewSlice(typed)
		analysis.Statistics = statisticsForObjects(typed, metadata.Fields)
	case map[string]any:
		metadata.TopLevel = "object"
		metadata.Rows = 1
		metadata.Fields = sortedKeys(typed)
		metadata.Columns = len(metadata.Fields)
		analysis.Preview = previewObject(typed, metadata.Fields)
		analysis.Statistics = statisticsForObject(typed, metadata.Fields)
	default:
		metadata.TopLevel = "scalar"
		metadata.Rows = 1
		analysis.Preview = typed
	}
	return metadata, analysis, nil
}

func collectObjectFields(rows []any) []string {
	seen := make(map[string]struct{})
	for _, row := range rows {
		object, ok := row.(map[string]any)
		if !ok {
			continue
		}
		for key := range object {
			seen[key] = struct{}{}
			if len(seen) >= maxStructuredFields {
				break
			}
		}
		if len(seen) >= maxStructuredFields {
			break
		}
	}
	return sortedKeySet(seen)
}
func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > maxStructuredFields {
		keys = keys[:maxStructuredFields]
	}
	return keys
}

func sortedKeySet(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func previewSlice(values []any) []any {
	limit := len(values)
	if limit > maxStructuredPreviewRows {
		limit = maxStructuredPreviewRows
	}
	preview := make([]any, 0, limit)
	for _, value := range values[:limit] {
		if object, ok := value.(map[string]any); ok {
			preview = append(preview, previewObject(object, sortedKeys(object)))
			continue
		}
		preview = append(preview, value)
	}
	return preview
}
func previewObject(object map[string]any, fields []string) map[string]any {
	preview := make(map[string]any, len(fields))
	for _, field := range fields {
		value, ok := object[field]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok && len(text) > 256 {
			value = text[:256] + "…"
		}
		preview[field] = value
	}
	return preview
}

func statisticsForObjects(rows []any, fields []string) map[string]numericSummary {
	accumulators := make(map[string]*numericAccumulator)
	for _, row := range rows {
		object, ok := row.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range fields {
			if value, ok := numericValue(object[field]); ok {
				acc := accumulators[field]
				if acc == nil {
					acc = &numericAccumulator{}
					accumulators[field] = acc
				}
				acc.add(value)
			}
		}
	}
	return finalizeStatistics(accumulators)
}
func statisticsForObject(object map[string]any, fields []string) map[string]numericSummary {
	accumulators := make(map[string]*numericAccumulator)
	for _, field := range fields {
		if value, ok := numericValue(object[field]); ok {
			acc := &numericAccumulator{}
			acc.add(value)
			accumulators[field] = acc
		}
	}
	return finalizeStatistics(accumulators)
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func finalizeStatistics(accumulators map[string]*numericAccumulator) map[string]numericSummary {
	if len(accumulators) == 0 {
		return nil
	}
	result := make(map[string]numericSummary, len(accumulators))
	for field, accumulator := range accumulators {
		result[field] = accumulator.summary()
	}
	return result
}
func analyzeJSONL(data []byte, truncated bool) (structuredMetadata, structuredAnalysis, error) {
	if truncated {
		if cut := bytes.LastIndexByte(data, '\n'); cut >= 0 {
			data = data[:cut+1]
		}
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), MaxReadFileBytes)
	rows := make([]any, 0)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return structuredMetadata{}, structuredAnalysis{}, err
		}
		rows = append(rows, value)
	}
	if err := scanner.Err(); err != nil {
		return structuredMetadata{}, structuredAnalysis{}, err
	}
	fields := collectObjectFields(rows)
	return structuredMetadata{
		Format: "jsonl", TopLevel: "records", Rows: len(rows), Columns: len(fields), Fields: fields,
	}, structuredAnalysis{Preview: previewSlice(rows), Statistics: statisticsForObjects(rows, fields)}, nil
}
func analyzeDelimited(data []byte, kind artifactKind, truncated bool) (structuredMetadata, structuredAnalysis, error) {
	if truncated {
		if cut := bytes.LastIndexByte(data, '\n'); cut >= 0 {
			data = data[:cut+1]
		}
	}
	reader := csv.NewReader(bytes.NewReader(data))
	if kind == artifactTSV {
		reader.Comma = '\t'
	}
	header, err := reader.Read()
	if err != nil {
		return structuredMetadata{}, structuredAnalysis{}, err
	}
	fields := append([]string(nil), header...)
	if len(fields) > maxStructuredFields {
		fields = fields[:maxStructuredFields]
	}
	preview := make([]map[string]string, 0, maxStructuredPreviewRows)
	accumulators := make(map[string]*numericAccumulator)
	rows := 0
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return structuredMetadata{}, structuredAnalysis{}, readErr
		}
		rows++
		if len(preview) < maxStructuredPreviewRows {
			row := make(map[string]string, len(fields))
			for index, field := range fields {
				if index < len(record) {
					row[field] = record[index]
				}
			}
			preview = append(preview, row)
		}
		for index, field := range fields {
			if index >= len(record) {
				continue
			}
			value, parseErr := strconv.ParseFloat(strings.TrimSpace(record[index]), 64)
			if parseErr != nil {
				continue
			}
			acc := accumulators[field]
			if acc == nil {
				acc = &numericAccumulator{}
				accumulators[field] = acc
			}
			acc.add(value)
		}
	}
	format := "csv"
	if kind == artifactTSV {
		format = "tsv"
	}
	return structuredMetadata{
		Format: format, TopLevel: "table", Rows: rows, Columns: len(header), Fields: fields,
	}, structuredAnalysis{Preview: preview, Statistics: finalizeStatistics(accumulators)}, nil
}
