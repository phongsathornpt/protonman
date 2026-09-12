// Package conversation owns bounded in-memory conversation retention.
package conversation

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/base/strutil"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// RetentionPolicy bounds live provider-neutral conversation history. Limits are
// soft only when the protected leading system messages plus the newest protocol
// group exceed a limit by themselves.
type RetentionPolicy struct {
	MaxMessages                  int
	MaxBytes                     int
	RecentMessages               int
	MaxHistoricalToolResultBytes int
}

// DefaultRetentionPolicy returns conservative live-history bounds. Persisted
// sessions have their own, smaller redacted retention policy.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		MaxMessages:                  runtimepolicy.ConversationMaxMessages,
		MaxBytes:                     runtimepolicy.ConversationMaxBytes,
		RecentMessages:               runtimepolicy.ConversationRecentMessages,
		MaxHistoricalToolResultBytes: runtimepolicy.ConversationHistoricalToolBytes,
	}
}

type messageSpan struct {
	start int
	end   int
	bytes int
}

// Retain returns protocol-safe bounded history. When no compaction or trimming
// is required it returns the input slice unchanged; callers that need ownership
// isolation should clone before calling. Assistant tool-call messages and their
// following tool results are retained or dropped as one unit.
func Retain(messages []sdk.Message, policy RetentionPolicy) []sdk.Message {
	if len(messages) == 0 {
		return nil
	}
	messages = compactHistoricalToolGroups(messages, policy.RecentMessages, policy.MaxHistoricalToolResultBytes)
	countExceeded := policy.MaxMessages > 0 && len(messages) > policy.MaxMessages
	retainedBytes := 0
	if !countExceeded && policy.MaxBytes > 0 {
		retainedBytes = EstimatedBytes(messages)
	}
	if !countExceeded && (policy.MaxBytes <= 0 || retainedBytes <= policy.MaxBytes) {
		return messages
	}
	leadingSystems := 0
	protectedBytes := 0
	for leadingSystems < len(messages) && messages[leadingSystems].Role == sdk.RoleSystem {
		protectedBytes += messageBytes(messages[leadingSystems])
		leadingSystems++
	}
	spans := buildSpans(messages, leadingSystems)
	if len(spans) == 0 {
		return append([]sdk.Message(nil), messages...)
	}

	retainedMessages := len(messages)
	retainedBytes = protectedBytes
	for _, span := range spans {
		retainedBytes += span.bytes
	}
	drop := 0
	for drop < len(spans)-1 && exceeds(policy, retainedMessages, retainedBytes) {
		span := spans[drop]
		retainedMessages -= span.end - span.start
		retainedBytes -= span.bytes
		drop++
	}

	start := spans[drop].start
	out := make([]sdk.Message, 0, leadingSystems+len(messages)-start)
	out = append(out, messages[:leadingSystems]...)
	out = append(out, messages[start:]...)
	return out
}

func exceeds(policy RetentionPolicy, messages, bytes int) bool {
	return policy.MaxMessages > 0 && messages > policy.MaxMessages ||
		policy.MaxBytes > 0 && bytes > policy.MaxBytes
}

func compactHistoricalToolGroups(messages []sdk.Message, recentMessages, resultLimit int) []sdk.Message {
	if recentMessages <= 0 || len(messages) <= recentMessages {
		return messages
	}
	leadingSystems := 0
	for leadingSystems < len(messages) && messages[leadingSystems].Role == sdk.RoleSystem {
		leadingSystems++
	}
	recentStart := len(messages) - recentMessages
	if recentStart < leadingSystems {
		recentStart = leadingSystems
	}
	for recentStart > leadingSystems && messages[recentStart].Role == sdk.RoleTool {
		recentStart--
	}
	if recentStart <= leadingSystems {
		return messages
	}
	hasCompactableToolHistory := false
	for index := leadingSystems; index < recentStart; index++ {
		message := messages[index]
		if message.Role == sdk.RoleTool || message.Role == sdk.RoleAssistant && len(message.ToolCalls) > 0 {
			hasCompactableToolHistory = true
			break
		}
	}
	if !hasCompactableToolHistory {
		return messages
	}

	out := make([]sdk.Message, 0, len(messages))
	out = append(out, messages[:leadingSystems]...)
	for index := leadingSystems; index < recentStart; {
		message := messages[index]
		if message.Role == sdk.RoleAssistant && len(message.ToolCalls) > 0 {
			if strings.TrimSpace(message.Content) != "" || len(message.Parts) > 0 {
				text := message
				text.ToolCalls = nil
				out = append(out, text)
			}
			next := index + 1
			for next < recentStart && messages[next].Role == sdk.RoleTool {
				next++
			}
			results := messages[index+1 : next]
			for _, call := range message.ToolCalls {
				resultIndex := toolResultIndex(results, call.ID)
				if resultIndex < 0 {
					out = append(out, historicalToolMessage(call.Name, sdk.Message{}, false, resultLimit))
					continue
				}
				out = append(out, historicalToolMessage(call.Name, results[resultIndex], true, resultLimit))
			}
			for _, result := range results {
				if toolCallIndex(message.ToolCalls, result.ToolCallID) >= 0 {
					continue
				}
				out = append(out, historicalToolMessage(result.ToolName, result, true, resultLimit))
			}
			index = next
			continue
		}
		if message.Role == sdk.RoleTool {
			out = append(out, historicalToolMessage(message.ToolName, message, true, resultLimit))
			index++
			continue
		}
		out = append(out, message)
		index++
	}
	out = append(out, messages[recentStart:]...)
	return out
}

func toolCallIndex(calls []sdk.ToolCall, callID string) int {
	for index, call := range calls {
		if call.ID == callID {
			return index
		}
	}
	return -1
}

func toolResultIndex(results []sdk.Message, callID string) int {
	for index, result := range results {
		if result.ToolCallID == callID {
			return index
		}
	}
	return -1
}

func historicalToolMessage(toolName string, message sdk.Message, found bool, limit int) sdk.Message {
	name := strings.TrimSpace(toolName)
	if name == "" {
		name = "unknown"
	}
	if !found {
		return sdk.Message{ID: sdk.NewMessageID(), Role: sdk.RoleAssistant, Content: fmt.Sprintf("Historical tool %s was requested, but its result is no longer retained.", name)}
	}
	return sdk.Message{ID: message.ID, Role: sdk.RoleAssistant, Content: historicalToolResultText(name, message.Content, limit)}
}

func historicalToolResultText(name, content string, limit int) string {
	if rawToolName, ok := canonicalJSONStringField(content, "tool_name"); ok {
		if decodedName, err := strconv.Unquote(rawToolName); err == nil {
			if canonical := strings.TrimSpace(decodedName); canonical != "" {
				name = canonical
			}
		}
	}
	if !canonicalJSONHasNonNullField(content, "error") {
		if rawOutput, ok := canonicalJSONStringField(content, "output"); ok {
			if output, ok := historicalOutputFromJSONString(name, rawOutput, limit); ok {
				return output
			}
		}
	}
	var result tool.Result
	if err := json.Unmarshal([]byte(content), &result); err == nil && (result.ToolName != "" || result.CallID != "" || result.Failure != nil) {
		if canonical := strings.TrimSpace(result.ToolName); canonical != "" {
			name = canonical
		}
		if result.Failure != nil {
			return truncateUTF8(fmt.Sprintf("Historical tool %s failed [%s]: %s", name, result.Failure.Code, result.Failure.Message), limit, "[historical tool output truncated]")
		}
		output := strings.TrimSpace(result.Output)
		if output == "" {
			output = strings.TrimSpace(strings.TrimSpace(result.Stdout) + "\n" + strings.TrimSpace(result.Stderr))
		}
		if output == "" && len(result.StructuredOutput) > 0 {
			output = strings.TrimSpace(string(result.StructuredOutput))
		}
		if output == "" {
			return fmt.Sprintf("Historical tool %s completed with no text output.", name)
		}
		return truncateUTF8(fmt.Sprintf("Historical tool %s result:\n%s", name, output), limit, "[historical tool output truncated]")
	}
	if text := strings.TrimSpace(content); text != "" {
		return truncateUTF8(fmt.Sprintf("Historical tool %s result:\n%s", name, text), limit, "[historical tool output truncated]")
	}
	return fmt.Sprintf("Historical tool %s completed with no text output.", name)
}

func canonicalJSONFieldValueStart(content, field string) (int, bool) {
	needle := `"` + field + `":`
	searchFrom := 0
	for searchFrom < len(content) {
		relative := strings.Index(content[searchFrom:], needle)
		if relative < 0 {
			return 0, false
		}
		index := searchFrom + relative
		before := index - 1
		for before >= 0 && (content[before] == ' ' || content[before] == '\t' || content[before] == '\n' || content[before] == '\r') {
			before--
		}
		if before < 0 || content[before] == '{' || content[before] == ',' {
			start := index + len(needle)
			for start < len(content) && (content[start] == ' ' || content[start] == '\t' || content[start] == '\n' || content[start] == '\r') {
				start++
			}
			return start, start < len(content)
		}
		searchFrom = index + len(needle)
	}
	return 0, false
}

func canonicalJSONStringField(content, field string) (string, bool) {
	start, ok := canonicalJSONFieldValueStart(content, field)
	if !ok || content[start] != '"' {
		return "", false
	}
	for end := start + 1; end < len(content); end++ {
		if content[end] == '\\' {
			end++
			continue
		}
		if content[end] == '"' {
			return content[start : end+1], true
		}
	}
	return "", false
}

func canonicalJSONHasNonNullField(content, field string) bool {
	start, ok := canonicalJSONFieldValueStart(content, field)
	if !ok {
		return false
	}
	return !strings.HasPrefix(content[start:], "null")
}

func historicalOutputFromJSONString(name, raw string, limit int) (string, bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}
	prefix := "Historical tool " + name + " result:\n"
	marker := "[historical tool output truncated]"
	decodedLen, valid := decodedJSONStringLen(raw)
	if !valid {
		return "", false
	}
	maxOutput := 0
	if limit > 0 {
		maxOutput = limit - len(prefix)
		if maxOutput <= 0 {
			return truncateUTF8(prefix, limit, marker), true
		}
	}
	willTruncate := maxOutput > 0 && decodedLen > maxOutput
	var out strings.Builder
	grow := len(prefix) + decodedLen
	if limit > 0 && grow > limit {
		grow = limit
	}
	if grow > 0 {
		out.Grow(grow)
	}
	out.WriteString(prefix)
	written := 0
	truncated := false
	rest := raw[1 : len(raw)-1]
	for rest != "" {
		escape := strings.IndexByte(rest, '\\')
		plain := rest
		if escape >= 0 {
			plain = rest[:escape]
		}
		if plain != "" {
			room := len(plain)
			if willTruncate {
				room = maxOutput - written - len(marker) - 1
			} else if maxOutput > 0 {
				room = maxOutput - written
			}
			if room < len(plain) {
				if room <= 0 {
					truncated = true
					break
				}
				cut := room
				for cut > 0 && !utf8.ValidString(plain[:cut]) {
					cut--
				}
				out.WriteString(plain[:cut])
				written += cut
				truncated = true
				break
			}
			out.WriteString(plain)
			written += len(plain)
			rest = rest[len(plain):]
		}
		if rest == "" {
			break
		}
		r, _, tail, err := strconv.UnquoteChar(rest, '"')
		if err != nil {
			return "", false
		}
		runeBytes := utf8.RuneLen(r)
		if runeBytes < 0 {
			runeBytes = 3
		}
		reserve := 0
		if willTruncate {
			reserve = len(marker) + 1
		}
		if maxOutput > 0 && written+runeBytes+reserve > maxOutput {
			truncated = true
			break
		}
		out.WriteRune(r)
		written += runeBytes
		rest = tail
	}
	if truncated {
		out.WriteByte('\n')
		out.WriteString(marker)
	}
	value := out.String()
	body := strings.TrimSpace(value[len(prefix):])
	if body == "" {
		return "", false
	}
	if len(body) == len(value)-len(prefix) {
		return value, true
	}
	return prefix + body, true
}

func decodedJSONStringLen(raw string) (int, bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return 0, false
	}
	rest := raw[1 : len(raw)-1]
	total := 0
	for rest != "" {
		escape := strings.IndexByte(rest, '\\')
		if escape < 0 {
			return total + len(rest), true
		}
		total += escape
		rest = rest[escape:]
		r, _, tail, err := strconv.UnquoteChar(rest, '"')
		if err != nil {
			return 0, false
		}
		runeBytes := utf8.RuneLen(r)
		if runeBytes < 0 {
			runeBytes = 3
		}
		total += runeBytes
		rest = tail
	}
	return total, true
}

func truncateUTF8(value string, limit int, marker string) string {
	return strutil.TruncateBytesWithMarker(value, limit, marker)
}

func buildSpans(messages []sdk.Message, start int) []messageSpan {
	spans := make([]messageSpan, 0, len(messages)-start)
	for index := start; index < len(messages); {
		end := index + 1
		if messages[index].Role == sdk.RoleAssistant && len(messages[index].ToolCalls) > 0 {
			for end < len(messages) && messages[end].Role == sdk.RoleTool {
				end++
			}
		}
		span := messageSpan{start: index, end: end}
		for i := index; i < end; i++ {
			span.bytes += messageBytes(messages[i])
		}
		spans = append(spans, span)
		index = end
	}
	return spans
}

func messageBytes(message sdk.Message) int {
	const messageOverhead = 128
	total := messageOverhead + len(message.Content) + len(message.ToolCallID) + len(message.ToolName)
	for _, part := range message.Parts {
		total += len(part.Type) + len(part.Text) + len(part.MIMEType) + len(part.Data)
	}
	for _, call := range message.ToolCalls {
		total += len(call.ID) + len(call.Name) + len(call.Arguments)
	}
	return total
}

// EstimatedBytes reports the retention payload estimate used by Retain.
func EstimatedBytes(messages []sdk.Message) int {
	total := 0
	for _, message := range messages {
		total += messageBytes(message)
	}
	return total
}
