package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

const maxCalculateExpressionBytes = 64 * 1024

type calculateHandler struct{}

type calculateInput struct {
	Expression string `json:"expression"`
}

type calculateOutput struct {
	Expression string  `json:"expression"`
	Value      float64 `json:"value"`
	Formatted  string  `json:"formatted"`
}

func NewCalculate() tool.Handler { return calculateHandler{} }

func (calculateHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:        "math",
		Description: "Evaluate deterministic numeric expressions locally in pure Go. Supports +, -, *, /, %, ^, parentheses, pi, e, and common functions such as sqrt, abs, min, max, pow, round, floor, ceil, ln, log10, exp, sin, cos, and tan.",
		Kind:        tool.KindCompute,
		Mutability:  tool.MutabilityReadOnly,
		Safety: tool.SafetyContract{
			MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone,
			CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone,
		},
		Evidence: tool.EvidenceNone,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{"type": "string", "description": "Numeric expression to evaluate"},
			},
			"required":             []string{"expression"},
			"additionalProperties": false,
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{"type": "string"},
				"value":      map[string]any{"type": "number"},
				"formatted":  map[string]any{"type": "string"},
			},
			"required":             []string{"expression", "value", "formatted"},
			"additionalProperties": false,
		},
	}
}

func (calculateHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{}, err
	}
	var input calculateInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode math arguments", err)
	}
	input.Expression = strings.TrimSpace(input.Expression)
	if input.Expression == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "math expression is required")
	}
	if len(input.Expression) > maxCalculateExpressionBytes {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "math expression exceeds 64 KiB")
	}
	parser := expressionParser{input: input.Expression}
	value, err := parser.parse()
	if err != nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "math: "+err.Error())
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "math result is not finite")
	}
	formatted := strconv.FormatFloat(value, 'g', -1, 64)
	payload := calculateOutput{Expression: input.Expression, Value: value, Formatted: formatted}
	structured, err := json.Marshal(payload)
	if err != nil {
		return tool.Result{}, fmt.Errorf("encode math result: %w", err)
	}
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: formatted, StructuredOutput: structured}, nil
}

type expressionParser struct {
	input string
	pos   int
}

func (p *expressionParser) parse() (float64, error) {
	value, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos != len(p.input) {
		return 0, fmt.Errorf("unexpected token at byte %d", p.pos)
	}
	return value, nil
}

func (p *expressionParser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.consume('+') {
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			left += right
		} else if p.consume('-') {
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			left -= right
		} else {
			return left, nil
		}
	}
}

func (p *expressionParser) parseTerm() (float64, error) {
	left, err := p.parsePower()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		switch {
		case p.consume('*'):
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			left *= right
		case p.consume('/'):
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
		case p.consume('%'):
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			left = math.Mod(left, right)
		default:
			return left, nil
		}
	}
}

func (p *expressionParser) parsePower() (float64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.consume('^') {
		right, err := p.parsePower()
		if err != nil {
			return 0, err
		}
		return math.Pow(left, right), nil
	}
	return left, nil
}

func (p *expressionParser) parseUnary() (float64, error) {
	p.skipSpace()
	if p.consume('+') {
		return p.parseUnary()
	}
	if p.consume('-') {
		v, err := p.parseUnary()
		return -v, err
	}
	return p.parsePrimary()
}

func (p *expressionParser) parsePrimary() (float64, error) {
	p.skipSpace()
	if p.consume('(') {
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if !p.consume(')') {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		return v, nil
	}
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("expected value")
	}
	if unicode.IsLetter(rune(p.input[p.pos])) {
		name := p.parseIdentifier()
		p.skipSpace()
		if !p.consume('(') {
			switch strings.ToLower(name) {
			case "pi":
				return math.Pi, nil
			case "e":
				return math.E, nil
			default:
				return 0, fmt.Errorf("unknown constant %q", name)
			}
		}
		args, err := p.parseArguments()
		if err != nil {
			return 0, err
		}
		return evalFunction(strings.ToLower(name), args)
	}
	return p.parseNumber()
}

func (p *expressionParser) parseArguments() ([]float64, error) {
	p.skipSpace()
	if p.consume(')') {
		return nil, nil
	}
	args := make([]float64, 0, 2)
	for {
		v, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, v)
		p.skipSpace()
		if p.consume(')') {
			return args, nil
		}
		if !p.consume(',') {
			return nil, fmt.Errorf("expected comma or closing parenthesis")
		}
	}
}

func (p *expressionParser) parseNumber() (float64, error) {
	start := p.pos
	seenDigit := false
	for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
		p.pos++
		seenDigit = true
	}
	if p.pos < len(p.input) && p.input[p.pos] == '.' {
		p.pos++
		for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
			p.pos++
			seenDigit = true
		}
	}
	if !seenDigit {
		return 0, fmt.Errorf("expected number at byte %d", start)
	}
	if p.pos < len(p.input) && (p.input[p.pos] == 'e' || p.input[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.input) && (p.input[p.pos] == '+' || p.input[p.pos] == '-') {
			p.pos++
		}
		expStart := p.pos
		for p.pos < len(p.input) && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
			p.pos++
		}
		if expStart == p.pos {
			return 0, fmt.Errorf("invalid exponent")
		}
	}
	v, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", p.input[start:p.pos])
	}
	return v, nil
}

func (p *expressionParser) parseIdentifier() string {
	start := p.pos
	for p.pos < len(p.input) {
		r := rune(p.input[p.pos])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *expressionParser) skipSpace() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}
func (p *expressionParser) consume(ch byte) bool {
	if p.pos < len(p.input) && p.input[p.pos] == ch {
		p.pos++
		return true
	}
	return false
}

func evalFunction(name string, args []float64) (float64, error) {
	one := func(fn func(float64) float64) (float64, error) {
		if len(args) != 1 {
			return 0, fmt.Errorf("%s expects 1 argument", name)
		}
		return fn(args[0]), nil
	}
	two := func(fn func(float64, float64) float64) (float64, error) {
		if len(args) != 2 {
			return 0, fmt.Errorf("%s expects 2 arguments", name)
		}
		return fn(args[0], args[1]), nil
	}
	switch name {
	case "sqrt":
		return one(math.Sqrt)
	case "abs":
		return one(math.Abs)
	case "round":
		return one(math.Round)
	case "floor":
		return one(math.Floor)
	case "ceil":
		return one(math.Ceil)
	case "ln", "log":
		return one(math.Log)
	case "log10":
		return one(math.Log10)
	case "exp":
		return one(math.Exp)
	case "sin":
		return one(math.Sin)
	case "cos":
		return one(math.Cos)
	case "tan":
		return one(math.Tan)
	case "pow":
		return two(math.Pow)
	case "min":
		if len(args) < 1 {
			return 0, fmt.Errorf("min expects at least 1 argument")
		}
		v := args[0]
		for _, x := range args[1:] {
			v = math.Min(v, x)
		}
		return v, nil
	case "max":
		if len(args) < 1 {
			return 0, fmt.Errorf("max expects at least 1 argument")
		}
		v := args[0]
		for _, x := range args[1:] {
			v = math.Max(v, x)
		}
		return v, nil
	default:
		return 0, fmt.Errorf("unknown function %q", name)
	}
}
