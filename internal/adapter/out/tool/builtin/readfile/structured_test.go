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
			Statistics map[string]numericSummary `json:"statistics"`
		} `json:"analysis"`
	}
	if err := json.Unmarshal(result.StructuredOutput, &got); err != nil {
		t.Fatalf("structured output: %v", err)
	}
	if got.Kind != "json" || got.Metadata.Rows != 2 || got.Metadata.Columns != 2 {
		t.Fatalf("structured json result = %+v", got)
	}
	latency := got.Analysis.Statistics["latency"]
	if latency.Count != 2 || latency.Min != 10 || latency.Max != 20 || latency.Mean != 15 {
		t.Fatalf("latency stats = %+v", latency)
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
