// Package conversation owns bounded in-memory conversation retention.
package conversation

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
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

// Retain returns a fresh top-level slice containing a protocol-safe bounded
// history. Assistant tool-call messages and their following tool results are
// retained or dropped as one unit so providers never receive half a tool group.
func Retain(messages []sdk.Message, policy RetentionPolicy) []sdk.Message {
	if len(messages) == 0 {
		return nil
	}
	messages = compactHistoricalToolGroups(messages, policy.RecentMessages, policy.MaxHistoricalToolResultBytes)
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
	retainedBytes := protectedBytes
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
			results := make(map[string]sdk.Message, len(message.ToolCalls))
			orderedResults := make([]sdk.Message, 0, len(message.ToolCalls))
			for next < recentStart && messages[next].Role == sdk.RoleTool {
				result := messages[next]
				results[result.ToolCallID] = result
				orderedResults = append(orderedResults, result)
				next++
			}
			for _, call := range message.ToolCalls {
				result, found := results[call.ID]
				out = append(out, historicalToolMessage(call.Name, result, found, resultLimit))
				delete(results, call.ID)
			}
			for _, result := range orderedResults {
				if _, ok := results[result.ToolCallID]; !ok {
					continue
				}
				out = append(out, historicalToolMessage(result.ToolName, result, true, resultLimit))
				delete(results, result.ToolCallID)
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

func historicalToolMessage(toolName string, message sdk.Message, found bool, limit int) sdk.Message {
	name := tool.CanonicalName(strings.TrimSpace(toolName))
	if name == "" {
		name = "unknown"
	}
	if !found {
		return sdk.Message{Role: sdk.RoleAssistant, Content: fmt.Sprintf("Historical tool %s was requested, but its result is no longer retained.", name)}
	}
	content := historicalToolResultText(name, message.Content)
	if limit > 0 {
		content = truncateUTF8(content, limit, "[historical tool output truncated]")
	}
	return sdk.Message{Role: sdk.RoleAssistant, Content: content}
}

func historicalToolResultText(name, content string) string {
	var result tool.Result
	if err := json.Unmarshal([]byte(content), &result); err == nil && (result.ToolName != "" || result.CallID != "" || result.Failure != nil) {
		if canonical := tool.CanonicalName(strings.TrimSpace(result.ToolName)); canonical != "" {
			name = canonical
		}
		if result.Failure != nil {
			return fmt.Sprintf("Historical tool %s failed [%s]: %s", name, result.Failure.Code, result.Failure.Message)
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
		return fmt.Sprintf("Historical tool %s result:\n%s", name, output)
	}
	if text := strings.TrimSpace(content); text != "" {
		return fmt.Sprintf("Historical tool %s result:\n%s", name, text)
	}
	return fmt.Sprintf("Historical tool %s completed with no text output.", name)
}

func truncateUTF8(value string, limit int, marker string) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if len(marker) >= limit {
		return marker[:limit]
	}
	prefixLimit := limit - len(marker) - 1
	value = value[:prefixLimit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimRight(value, "\n") + "\n" + marker
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
