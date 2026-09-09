// Package conversation owns bounded in-memory conversation retention.
package conversation

import (
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// RetentionPolicy bounds live provider-neutral conversation history. Limits are
// soft only when the protected leading system messages plus the newest protocol
// group exceed a limit by themselves.
type RetentionPolicy struct {
	MaxMessages int
	MaxBytes    int
}

// DefaultRetentionPolicy returns conservative live-history bounds. Persisted
// sessions have their own, smaller redacted retention policy.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		MaxMessages: runtimepolicy.ConversationMaxMessages,
		MaxBytes:    runtimepolicy.ConversationMaxBytes,
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
