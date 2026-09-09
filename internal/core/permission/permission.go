// Package permission defines the policy and decision model for tool access.
package permission

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/internal/base/glob"
	"github.com/phongsathornpt/protonman/internal/core/tool"
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

// Valid reports whether the action is a supported non-zero decision.
func (a Action) Valid() bool {
	return validAction(a)
}

// MarshalText implements encoding.TextMarshaler.
func (a Action) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (a *Action) UnmarshalText(text []byte) error {
	parsed, err := ParseAction(string(text))
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

// ParseAction parses the action names used by configuration files.
func ParseAction(value string) (Action, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "allow":
		return ActionAllow, nil
	case "deny":
		return ActionDeny, nil
	case "ask":
		return ActionAsk, nil
	default:
		return ActionUnknown, fmt.Errorf("unknown permission action %q", value)
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

// Valid reports whether the mode is a supported non-zero mode.
func (m Mode) Valid() bool {
	switch m {
	case ModeAsk, ModeAuto, ModeAlwaysApprove, ModeDeny:
		return true
	default:
		return false
	}
}

// MarshalText implements encoding.TextMarshaler.
func (m Mode) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *Mode) UnmarshalText(text []byte) error {
	parsed, err := ParseMode(string(text))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
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
	case "deny", "dont-ask", "dont_ask", "plan":
		return ModeDeny, nil
	default:
		return ModeUnknown, fmt.Errorf("unknown permission mode %q", value)
	}
}

// ToolKind identifies the type of access a permission rule governs.
type ToolKind = tool.Kind

const (
	// ToolAny matches every tool kind.
	ToolAny ToolKind = "any"
	// ToolRead matches read-only filesystem tools.
	ToolRead ToolKind = tool.KindRead
	// ToolEdit matches file-mutating tools.
	ToolEdit ToolKind = tool.KindEdit
	// ToolBash matches shell execution tools.
	ToolBash ToolKind = tool.KindBash
	// ToolGrep matches content-search tools.
	ToolGrep ToolKind = tool.KindGrep
	// ToolGit matches repository operations.
	ToolGit ToolKind = tool.KindGit
	// ToolMCP matches tools supplied by MCP servers.
	ToolMCP ToolKind = tool.KindMCP
	// ToolWeb matches URL-fetching tools.
	ToolWeb ToolKind = tool.KindWeb
	// ToolTask matches structured planning/task metadata tools.
	ToolTask ToolKind = tool.KindTask
	// ToolAgent matches subagent orchestration tools.
	ToolAgent ToolKind = tool.KindAgent
	// ToolCompute matches deterministic local computation tools.
	ToolCompute ToolKind = tool.KindCompute
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

// String returns the stable configuration spelling of a pattern mode.
func (m PatternMode) String() string {
	switch m {
	case PatternModeGlob:
		return "glob"
	case PatternModeDomain:
		return "domain"
	default:
		return "unknown"
	}
}

// Valid reports whether the pattern mode is a recognized non-zero mode.
func (m PatternMode) Valid() bool {
	return m == PatternModeGlob || m == PatternModeDomain
}

// MarshalText implements encoding.TextMarshaler.
func (m PatternMode) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *PatternMode) UnmarshalText(text []byte) error {
	parsed, err := ParsePatternMode(string(text))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// ParseToolKind parses the tool names used by configuration files.
func ParseToolKind(value string) (ToolKind, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "any":
		return ToolAny, nil
	case "read":
		return ToolRead, nil
	case "edit":
		return ToolEdit, nil
	case "bash", "shell":
		return ToolBash, nil
	case "grep", "search":
		return ToolGrep, nil
	case "git":
		return ToolGit, nil
	case "mcp":
		return ToolMCP, nil
	case "web":
		return ToolWeb, nil
	case "task", "todo":
		return ToolTask, nil
	case "agent", "subagent":
		return ToolAgent, nil
	case "compute", "math":
		return ToolCompute, nil
	default:
		return "", fmt.Errorf("unknown permission tool %q", value)
	}
}

// ParsePatternMode parses the pattern modes used by configuration files.
func ParsePatternMode(value string) (PatternMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "glob":
		return PatternModeGlob, nil
	case "domain":
		return PatternModeDomain, nil
	default:
		return PatternModeUnknown, fmt.Errorf("unknown permission pattern mode %q", value)
	}
}

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
	// Risk records proven destructive command behavior independently from generic mutability.
	Risk tool.CommandRisk
	// Effect records the per-call shell effect. Unknown fails closed:
	// session-grant reuse requires Effect == read_only.
	Effect tool.CommandEffect
	// Scope distinguishes local edits from remote, publish, and deployment mutations.
	Scope tool.CommandScope
}

// Decision is the result of evaluating a static policy.
type Decision struct {
	// Action is allow, deny, or ask.
	Action Action
	// Reason explains the winning rule or default.
	Reason string
}

// GrantScope determines how long an interactive approval remains effective.
type GrantScope uint8

const (
	// GrantScopeUnknown is the invalid zero value.
	GrantScopeUnknown GrantScope = iota
	// GrantScopeOnce applies only to the current call.
	GrantScopeOnce
	// GrantScopeSession applies to matching calls until the process exits.
	GrantScopeSession
)

// String returns the stable spelling of a grant scope.
func (g GrantScope) String() string {
	switch g {
	case GrantScopeOnce:
		return "once"
	case GrantScopeSession:
		return "session"
	default:
		return "unknown"
	}
}

// Valid reports whether the grant scope is a recognized non-zero scope.
func (g GrantScope) Valid() bool {
	return g == GrantScopeOnce || g == GrantScopeSession
}

// ParseGrantScope parses the grant scope names.
func ParseGrantScope(value string) (GrantScope, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "once":
		return GrantScopeOnce, nil
	case "session":
		return GrantScopeSession, nil
	default:
		return GrantScopeUnknown, fmt.Errorf("unknown permission grant scope %q", value)
	}
}

// MarshalText implements encoding.TextMarshaler.
func (g GrantScope) MarshalText() ([]byte, error) {
	return []byte(g.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (g *GrantScope) UnmarshalText(text []byte) error {
	parsed, err := ParseGrantScope(string(text))
	if err != nil {
		return err
	}
	*g = parsed
	return nil
}

// GrantKey identifies the narrow request a session grant covers.
type GrantKey struct {
	// ToolName prevents a grant for one tool from authorizing another tool.
	ToolName string
	// ToolKind keeps policy categories explicit in the key.
	ToolKind ToolKind
	// Detail is the user-visible target approved by the user.
	Detail string
	// ArgumentsSHA256 binds the grant to the normalized argument payload, not
	// merely its presentation detail.
	ArgumentsSHA256 [32]byte
	// Effect, Risk, and Scope prevent authorization reuse if the same visible
	// target is later classified differently.
	Effect tool.CommandEffect
	Risk   tool.CommandRisk
	Scope  tool.CommandScope
}

// SessionGrantEligible reports whether a request is safe to remember across
// later calls in the same process. Unknown, mutating, or destructive requests
// remain one-shot even when an interactive resolver asks for session scope.
func SessionGrantEligible(r Request) bool {
	return r.Effect == tool.CommandEffectReadOnly && r.Risk == tool.CommandRiskNormal
}

// PersistentRuleEligible reports whether a request is safe to remember across
// future runs by persisting a static allow rule to project or user configuration.
// Mutating, unknown, or destructive requests remain one-shot or session-bound.
func PersistentRuleEligible(r Request) bool {
	if !SessionGrantEligible(r) {
		return false
	}
	_, ok := RuleFromRequest(r)
	return ok
}

// RuleFromRequest constructs a static allow Rule that matches future occurrences
// of the given request. It returns false if the request cannot be converted
// into a safe, specific rule (e.g. empty target, multi-line commands, raw JSON).
func RuleFromRequest(r Request) (Rule, bool) {
	if r.ToolKind == ToolMCP {
		name := strings.TrimSpace(r.ToolName)
		if name == "" {
			return Rule{}, false
		}
		return Rule{
			Action:      ActionAllow,
			Tool:        ToolMCP,
			Pattern:     name,
			PatternMode: PatternModeGlob,
		}, true
	}

	detail := strings.TrimSpace(r.Detail)
	if detail == "" || strings.ContainsAny(detail, "\r\n") {
		return Rule{}, false
	}
	if strings.HasPrefix(detail, "{") && strings.HasSuffix(detail, "}") {
		return Rule{}, false
	}

	switch r.ToolKind {
	case ToolBash:
		return Rule{
			Action:      ActionAllow,
			Tool:        ToolBash,
			Pattern:     detail,
			PatternMode: PatternModeGlob,
		}, true

	case ToolRead, ToolGrep, ToolGit:
		return Rule{
			Action:      ActionAllow,
			Tool:        r.ToolKind,
			Pattern:     detail,
			PatternMode: PatternModeGlob,
		}, true

	case ToolWeb:
		parsed, err := url.Parse(detail)
		if err != nil || parsed.Hostname() == "" {
			return Rule{}, false
		}
		return Rule{
			Action:      ActionAllow,
			Tool:        ToolWeb,
			Pattern:     strings.ToLower(parsed.Hostname()),
			PatternMode: PatternModeDomain,
		}, true

	default:
		return Rule{}, false
	}
}

// Key returns the exact session-grant key for a request.
func (r Request) Key() GrantKey {
	return GrantKey{
		ToolName:        r.ToolName,
		ToolKind:        r.ToolKind,
		Detail:          r.Detail,
		ArgumentsSHA256: sha256.Sum256(r.Arguments),
		Effect:          r.Effect,
		Risk:            r.Risk,
		Scope:           r.Scope,
	}
}

// Resolution is the final result returned by an interactive prompt.
type Resolution struct {
	// Action must be allow or deny.
	Action Action
	// Scope controls whether an allow is one-shot or remembered for this
	// exact request during the current process.
	Scope GrantScope
	// Reason is displayed in diagnostics and tests.
	Reason string
}

// ErrInvalidConfig indicates a policy cannot be constructed safely.
var ErrInvalidConfig = errors.New("invalid permission config")

// Policy evaluates static rules without performing I/O or prompting.
type Policy struct {
	mu            sync.RWMutex
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
		if rule.Tool == "" {
			rule.Tool = ToolAny
			rules[i].Tool = ToolAny
		}
		if !validToolKind(rule.Tool) {
			return nil, fmt.Errorf("%w: rule %d has invalid tool %q", ErrInvalidConfig, i, rule.Tool)
		}
		patternMode := rule.PatternMode
		if patternMode == PatternModeUnknown {
			patternMode = PatternModeGlob
			rules[i].PatternMode = patternMode
		}
		if !patternMode.Valid() {
			return nil, fmt.Errorf("%w: rule %d has invalid pattern mode %s", ErrInvalidConfig, i, patternMode)
		}
		if patternMode == PatternModeDomain {
			if strings.EqualFold(strings.TrimSpace(rule.Pattern), "all") {
				rules[i].Pattern = "*"
			} else {
				rules[i].Pattern = strings.ToLower(strings.TrimSpace(rule.Pattern))
			}
		} else if rule.Tool == ToolMCP {
			pattern := strings.TrimSpace(rule.Pattern)
			// Dotted MCP patterns are tool namespaces. Plain patterns remain
			// request-detail matchers for backwards-compatible resource rules.
			if strings.EqualFold(pattern, "all") {
				pattern = "*"
			} else if pattern != "" && strings.Contains(pattern, ".") && !strings.HasPrefix(pattern, "mcp.") {
				pattern = "mcp." + pattern
			}
			rules[i].Pattern = pattern
		} else {
			pattern := strings.TrimSpace(rule.Pattern)
			if strings.EqualFold(pattern, "all") {
				pattern = "*"
			}
			rules[i].Pattern = pattern
		}
	}

	return &Policy{
		rules:         rules,
		defaultAction: defaultAction,
	}, nil
}

// Evaluate returns the effective static decision for a request.
func (p *Policy) Evaluate(request Request) Decision {
	p.mu.RLock()
	defer p.mu.RUnlock()
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
	if request.ToolKind == ToolCompute {
		return Decision{Action: ActionAllow, Reason: "safe local computation"}
	}
	return Decision{
		Action: p.defaultAction,
		Reason: "permission policy default: " + p.defaultAction.String(),
	}
}

// AddRule appends a static rule to the policy in a thread-safe manner.
// If an identical rule already exists, AddRule is a no-op.
func (p *Policy) AddRule(rule Rule) error {
	if !validAction(rule.Action) {
		return fmt.Errorf("%w: invalid action %d", ErrInvalidConfig, rule.Action)
	}
	if rule.Tool == "" {
		rule.Tool = ToolAny
	}
	if !validToolKind(rule.Tool) {
		return fmt.Errorf("%w: invalid tool %q", ErrInvalidConfig, rule.Tool)
	}
	patternMode := rule.PatternMode
	if patternMode == PatternModeUnknown {
		patternMode = PatternModeGlob
	}
	if !patternMode.Valid() {
		return fmt.Errorf("%w: invalid pattern mode %s", ErrInvalidConfig, patternMode)
	}
	rule.PatternMode = patternMode
	rule.Pattern = NormalizePattern(rule.Tool, rule.PatternMode, rule.Pattern)

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, existing := range p.rules {
		if existing.Action == rule.Action &&
			existing.Tool == rule.Tool &&
			existing.Pattern == rule.Pattern &&
			existing.PatternMode == rule.PatternMode {
			return nil
		}
	}
	p.rules = append(p.rules, rule)
	return nil
}

// Rules returns a defensive copy of the current policy rules.
func (p *Policy) Rules() []Rule {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Rule, len(p.rules))
	copy(out, p.rules)
	return out
}

func ruleMatches(rule Rule, request Request) bool {
	if rule.Tool != ToolAny && rule.Tool != request.ToolKind {
		return false
	}
	if rule.Pattern == "" || rule.Pattern == "*" {
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
	if glob.Match(rule.Pattern, detail) {
		return true
	}
	if request.ToolKind == ToolMCP && glob.Match(rule.Pattern, request.ToolName) {
		return true
	}
	return false
}

func ruleReason(action string, rule Rule, request Request) string {
	pattern := rule.Pattern
	if pattern == "" {
		pattern = "*"
	}
	return fmt.Sprintf("%s by %s rule for %s matching %q", action, rule.Tool, request.ToolName, pattern)
}

func validAction(action Action) bool {
	return action == ActionAllow || action == ActionDeny || action == ActionAsk
}

// ValidToolKind reports whether a tool kind is supported by permission policy.
func ValidToolKind(kind ToolKind) bool {
	return validToolKind(kind)
}

func validToolKind(kind ToolKind) bool {
	switch kind {
	case ToolAny, ToolRead, ToolEdit, ToolBash, ToolGrep, ToolGit, ToolMCP, ToolWeb, ToolTask, ToolAgent, ToolCompute:
		return true
	default:
		return false
	}
}

// NormalizePattern returns the canonical pattern for a tool kind and pattern mode.
// It normalizes "all" and empty patterns to "*" for wildcard matching,
// canonicalizes MCP namespaces, and lowercases domain patterns.
func NormalizePattern(kind ToolKind, patternMode PatternMode, pattern string) string {
	trimmed := strings.TrimSpace(pattern)
	if patternMode == PatternModeDomain {
		if trimmed == "" || trimmed == "*" || strings.EqualFold(trimmed, "all") {
			return "*"
		}
		return strings.ToLower(trimmed)
	}
	if kind == ToolMCP {
		if strings.EqualFold(trimmed, "all") {
			return "*"
		}
		if trimmed != "" && strings.Contains(trimmed, ".") && !strings.HasPrefix(trimmed, "mcp.") {
			trimmed = "mcp." + trimmed
		}
	}
	if trimmed == "" || trimmed == "*" || strings.EqualFold(trimmed, "all") {
		return "*"
	}
	return trimmed
}
