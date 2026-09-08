package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestCalculateEvaluatesExpressions(t *testing.T) {
	tests := []struct {
		expression string
		want       float64
	}{
		{"2 + 3 * 4", 14},
		{"2^3^2", 512},
		{"sqrt(81) + max(2, 7, 4)", 16},
		{"round(10/3 * 100) / 100", 3.33},
		{"sin(pi / 2)", 1},
	}
	for _, tt := range tests {
		t.Run(tt.expression, func(t *testing.T) {
			result, err := NewCalculate().Execute(context.Background(), newJSONCall(t, "calc", "math", map[string]any{"expression": tt.expression}))
			if err != nil {
				t.Fatal(err)
			}
			var out calculateOutput
			if err := json.Unmarshal(result.StructuredOutput, &out); err != nil {
				t.Fatal(err)
			}
			if math.Abs(out.Value-tt.want) > 1e-12 {
				t.Fatalf("value = %v, want %v", out.Value, tt.want)
			}
		})
	}
}

func TestCalculateRejectsInvalidAndNonFiniteExpressions(t *testing.T) {
	for _, expression := range []string{"", "1/0", "sqrt(-1)", "unknown(1)", "1 +"} {
		t.Run(expression, func(t *testing.T) {
			_, err := NewCalculate().Execute(context.Background(), newJSONCall(t, "calc", "math", map[string]any{"expression": expression}))
			if err == nil {
				t.Fatal("expected error")
			}
			var toolErr *tool.ToolError
			if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeInvalidArguments {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
