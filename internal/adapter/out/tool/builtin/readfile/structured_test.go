package readfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileAutoAnalyzesJSON(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	data := `[{"model":"a","latency":10},{"model":"b","latency":20}]`
	if err := os.WriteFile(filepath.Join(ws.Root(), "bench.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "json", "read_file", map[string]any{"path": "bench.json", "view": "structured"}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "structured json · rows 2") {
		t.Fatalf("Output = %q", result.Output)
	}
	var got struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Rows    int      `json:"rows"`
			Columns int      `json:"columns"`
			Fields  []string `json:"fields"`
		} `json:"metadata"`
		Analysis struct {
			Statistics map[string]numericSummary      `json:"statistics"`
			Fields     map[string]fieldProfileSummary `json:"fields"`
		} `json:"analysis"`
	}
	if err := json.Unmarshal(result.StructuredOutput, &got); err != nil {
		t.Fatalf("structured output: %v", err)
	}
	if got.Kind != "json" || got.Metadata.Rows != 2 || got.Metadata.Columns != 2 {
		t.Fatalf("structured json result = %+v", got)
	}
	latency := got.Analysis.Statistics["latency"]
	if latency.Count != 2 || latency.Min != 10 || latency.Max != 20 || latency.Mean != 15 || latency.Median != 15 {
		t.Fatalf("latency stats = %+v", latency)
	}
	if got.Analysis.Fields["latency"].Type != "number" || got.Analysis.Fields["latency"].Unique != 2 {
		t.Fatalf("latency profile = %+v", got.Analysis.Fields["latency"])
	}
	if got.Analysis.Fields["model"].Type != "string" || got.Analysis.Fields["model"].Unique != 2 {
		t.Fatalf("model profile = %+v", got.Analysis.Fields["model"])
	}
}

func TestReadFileAutoAnalyzesCSV(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	data := "model,latency\na,10\nb,20\n"
	if err := os.WriteFile(filepath.Join(ws.Root(), "bench.csv"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "csv", "read_file", map[string]any{"path": "bench.csv", "view": "structured"}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "structured csv · rows 2 · columns 2") {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestReadFileAutoPreservesStructuredTextCompatibility(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	data := `{"name":"protonman","enabled":true}`
	if err := os.WriteFile(filepath.Join(ws.Root(), "config.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "json-text", "read_file", map[string]any{"path": "config.json"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != data || len(result.StructuredOutput) != 0 {
		t.Fatalf("auto JSON read = output %q structured %s", result.Output, result.StructuredOutput)
	}
}

func TestReadFileStructuredCSVInfersTypesAndMedian(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	data := "model,latency,enabled\na,10,true\nb,20,false\nc,30,true\n"
	if err := os.WriteFile(filepath.Join(ws.Root(), "typed.csv"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "csv-types", "read_file", map[string]any{
		"path": "typed.csv", "view": "structured",
	}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Analysis struct {
			Statistics map[string]numericSummary      `json:"statistics"`
			Fields     map[string]fieldProfileSummary `json:"fields"`
		} `json:"analysis"`
	}
	if err := json.Unmarshal(result.StructuredOutput, &got); err != nil {
		t.Fatal(err)
	}
	if stats := got.Analysis.Statistics["latency"]; stats.Median != 20 || stats.StdDev != 10 {
		t.Fatalf("latency stats = %+v", stats)
	}
	if profile := got.Analysis.Fields["latency"]; profile.Type != "number" || profile.Unique != 3 {
		t.Fatalf("latency profile = %+v", profile)
	}
	if profile := got.Analysis.Fields["enabled"]; profile.Type != "boolean" || profile.Unique != 2 {
		t.Fatalf("enabled profile = %+v", profile)
	}
	if profile := got.Analysis.Fields["model"]; profile.Type != "string" || profile.Unique != 3 {
		t.Fatalf("model profile = %+v", profile)
	}
}

func TestReadFileStructuredJSONProfilesNullableAndMixedFields(t *testing.T) {
	metadata, analysis, err := analyzeJSON([]byte(`[{"value":1},{"value":null},{"value":"x"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Rows != 3 {
		t.Fatalf("rows = %d, want 3", metadata.Rows)
	}
	profile := analysis.Fields["value"]
	if profile.Type != "mixed" || profile.NonNull != 2 || profile.Unique != 2 {
		t.Fatalf("value profile = %+v", profile)
	}
}
