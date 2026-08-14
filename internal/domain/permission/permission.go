// Package permission defines the policy and decision model for tool access.
package permission

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Action is the result of evaluating a permission rule.
type Action uint8

const (
	// ActionUnknown is the invalid zero value.
	ActionUnknown Action = iota
	// ActionAllow authorizes execution without a prompt.
	ActionAllow
	// ActionDeny blocks execution.
	ActionDeny
	// ActionAsk requires a user or policy resolver decision.
	ActionAsk
)

// String returns the stable configuration spelling of an action.
func (a Action) String() string {
	switch a {
	case ActionAllow:
		return "allow"
	case ActionDeny:
		return "deny"
	case ActionAsk:
		return "ask"
	default:
		return "unknown"
	}
}

// Mode controls what happens after static policy evaluation returns ask.
type Mode uint8

const (
	// ModeUnknown is the invalid zero value.
	ModeUnknown Mode = iota
	// ModeAsk prompts for calls not statically allowed.
	ModeAsk
	// ModeAuto is reserved for the future classifier-backed mode. The first
	// port uses the safe interactive prompt until that classifier exists.
	ModeAuto
	// ModeAlwaysApprove allows calls not explicitly denied by policy.
	ModeAlwaysApprove
	// ModeDeny denies calls not explicitly allowed by policy.
	ModeDeny
)

// String returns the stable configuration spelling of a mode.
func (m Mode) String() string {
	switch m {
	case ModeAsk:
		return "ask"
	case ModeAuto:
		return "auto"
	case ModeAlwaysApprove:
		return "always-approve"
	case ModeDeny:
		return "deny"
	default:
		return "unknown"
	}
}

// ParseMode parses the mode names accepted by the TUI and future config.
func ParseMode(value string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ask":
		return ModeAsk, nil
	case "auto":
		return ModeAuto, nil
	case "always-approve", "always_approve", "yolo":
		return ModeAlwaysApprove, nil
	case "deny", "dont-ask", "dont_ask":
		return ModeDeny, nil
	default:
		return ModeUnknown, fmt.Errorf("unknown permission mode %q", value)
	}
}

// ToolKind identifies the type of access a permission rule governs.
type ToolKind string

const (
	// ToolAny matches every tool kind.
	ToolAny ToolKind = "any"
	// ToolRead matches read-only filesystem tools.
	ToolRead ToolKind = "read"
	// ToolEdit matches file-mutating tools.
	ToolEdit ToolKind = "edit"
	// ToolBash matches shell execution tools.
	ToolBash ToolKind = "bash"
	// ToolGrep matches content-search tools.
	ToolGrep ToolKind = "grep"
	// ToolMCP matches tools supplied by MCP servers.
	ToolMCP ToolKind = "mcp"
	// ToolWebFetch matches URL-fetching tools.
	ToolWebFetch ToolKind = "web_fetch"
	// ToolWebSearch matches web-search tools.
	ToolWebSearch ToolKind = "web_search"
)

// PatternMode controls what part of a request a rule pattern matches.
type PatternMode uint8

const (
	// PatternModeUnknown uses the default glob behavior when a rule is built
	// without an explicit mode.
	PatternModeUnknown PatternMode = iota
	// PatternModeGlob matches the request detail with * and ? wildcards.
	PatternModeGlob
	// PatternModeDomain matches a URL hostname, case-insensitively.
	PatternModeDomain
)

// Rule is one static permission rule.
type Rule struct {
	// Action is the decision when the rule matches.
	Action Action
	// Tool narrows the rule to a permission category.
	Tool ToolKind
	// Pattern is matched against request detail. Empty matches all details.
	Pattern string
	// PatternMode selects glob or URL-host matching.
	PatternMode PatternMode
}

// Config is the in-memory policy configuration.
type Config struct {
	// Rules are evaluated with deny > ask > allow precedence, independent of
	// their order in the slice.
	Rules []Rule
	// Default applies when no rule matches. The zero value becomes ask.
	Default Action
}

// Request is the context a permission policy evaluates.
type Request struct {
	// CallID identifies the call being authorized.
	CallID string
	// ToolName identifies the concrete tool.
	ToolName string
	// ToolKind is the policy category for the tool.
	ToolKind ToolKind
	// Detail is a command, path, URL, or similar user-visible target.
	Detail string
	// Arguments preserves the original JSON for future classifier-backed
	// decisions and richer prompts.
	Arguments json.RawMessage
}

// Decision is the result of evaluating a static policy.
type Decision struct {
	// Action is allow, deny, or ask.
	Action Action
	// Reason explains the winning rule or default.
	Reason string
}

// Resolution is the final result returned by an interactive prompt.
type Resolution struct {
	// Action must be allow or deny.
	Action Action
	// Remember changes the current process mode to always-approve when the
	// action is allow.
	Remember bool
	// Reason is displayed in diagnostics and tests.
	Reason string
}

// ErrInvalidConfig indicates a policy cannot be constructed safely.
var ErrInvalidConfig = errors.New("invalid permission config")

// Policy evaluates static rules without performing I/O or prompting.
type Policy struct {
	rules         []Rule
	defaultAction Action
}

// NewPolicy validates and compiles a permission policy.
func NewPolicy(config Config) (*Policy, error) {
	defaultAction := config.Default
	if defaultAction == ActionUnknown {
		defaultAction = ActionAsk
	}
	if !validAction(defaultAction) {
		return nil, fmt.Errorf("%w: invalid default action %d", ErrInvalidConfig, defaultAction)
	}

	rules := make([]Rule, len(config.Rules))
	copy(rules, config.Rules)
	for i, rule := range rules {
		if !validAction(rule.Action) {
			return nil, fmt.Errorf("%w: rule %d has invalid action %d", ErrInvalidConfig, i, rule.Action)
		}
		if !validToolKind(rule.Tool) {
			return nil, fmt.Errorf("%w: rule %d has invalid tool %q", ErrInvalidConfig, i, rule.Tool)
		}
		patternMode := rule.PatternMode
		if patternMode == PatternModeUnknown {
			patternMode = PatternModeGlob
			rules[i].PatternMode = patternMode
		}
		if patternMode != PatternModeGlob && patternMode != PatternModeDomain {
			return nil, fmt.Errorf("%w: rule %d has invalid pattern mode %d", ErrInvalidConfig, i, patternMode)
		}
	}

	return &Policy{
		rules:         rules,
		defaultAction: defaultAction,
	}, nil
}

// Evaluate returns the effective static decision for a request.
func (p *Policy) Evaluate(request Request) Decision {
	matchedAsk := false
	matchedAllow := false
	for _, rule := range p.rules {
		if !ruleMatches(rule, request) {
			continue
		}
		switch rule.Action {
		case ActionDeny:
			return Decision{
				Action: ActionDeny,
				Reason: ruleReason("denied", rule, request),
			}
		case ActionAsk:
			matchedAsk = true
		case ActionAllow:
			matchedAllow = true
		}
	}

	if matchedAsk {
		return Decision{Action: ActionAsk, Reason: "permission policy requires approval"}
	}
	if matchedAllow {
		return Decision{Action: ActionAllow, Reason: "allowed by permission policy"}
	}
	return Decision{
		Action: p.defaultAction,
		Reason: "permission policy default: " + p.defaultAction.String(),
	}
}

func ruleMatches(rule Rule, request Request) bool {
	if rule.Tool != ToolAny && rule.Tool != request.ToolKind {
		return false
	}
	if rule.Pattern == "" {
		return true
	}

	detail := request.Detail
	if rule.PatternMode == PatternModeDomain {
		parsed, err := url.Parse(detail)
		if err != nil || parsed.Hostname() == "" {
			return false
		}
		detail = strings.ToLower(parsed.Hostname())
	}
	return globMatch(rule.Pattern, detail)
}

func ruleReason(action string, rule Rule, request Request) string {
	pattern := rule.Pattern
	if pattern == "" {
		pattern = "*"
	}
	return fmt.Sprintf("%s by %s rule for %s matching %q", action, rule.Tool, request.ToolName, pattern)
}

func globMatch(pattern string, value string) bool {
	p := []rune(pattern)
	v := []rune(value)
	previous := make([]bool, len(v)+1)
	previous[0] = true

	for _, patternRune := range p {
		current := make([]bool, len(v)+1)
		switch patternRune {
		case '*':
			current[0] = previous[0]
			for valueIndex := 1; valueIndex <= len(v); valueIndex++ {
				current[valueIndex] = current[valueIndex-1] || previous[valueIndex]
			}
		case '?':
			for valueIndex := 1; valueIndex <= len(v); valueIndex++ {
				current[valueIndex] = previous[valueIndex-1]
			}
		default:
			for valueIndex, valueRune := range v {
				if valueRune == patternRune {
					current[valueIndex+1] = previous[valueIndex]
				}
			}
		}
		previous = current
	}
	return previous[len(v)]
}

func validAction(action Action) bool {
	return action == ActionAllow || action == ActionDeny || action == ActionAsk
}

func validToolKind(kind ToolKind) bool {
	switch kind {
	case ToolAny, ToolRead, ToolEdit, ToolBash, ToolGrep, ToolMCP, ToolWebFetch, ToolWebSearch:
		return true
	default:
		return false
	}
}
