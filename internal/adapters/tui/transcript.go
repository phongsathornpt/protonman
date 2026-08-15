package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	applicationturn "github.com/projectTHORN/proton/internal/application/turn"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

const maxBubbleScrollback = 1000

type blockKind uint8

const (
	blockUser blockKind = iota
	blockAssistant
	blockTool
	blockSystem
	blockError
)

// Block is one typed transcript entry.
type Block struct {
	Kind    blockKind
	Title   string
	Body    string
	Running bool
	Code    string
}

func (m *bubbleModel) pushBlock(block Block) {
	m.blocks = append(m.blocks, block)
	m.trimBlocks()
}

func (m *bubbleModel) trimBlocks() {
	for lineCount(m.blocks) > maxBubbleScrollback && len(m.blocks) > 1 {
		m.blocks = m.blocks[1:]
	}
}

func lineCount(blocks []Block) int {
	total := 0
	for _, block := range blocks {
		total += 1 + strings.Count(block.Body, "\n")
		if block.Title != "" && block.Kind == blockTool {
			total++
		}
	}
	return total
}

func (m *bubbleModel) appendLine(line string) {
	m.pushBlock(Block{Kind: blockSystem, Body: line})
}

func (m *bubbleModel) appendUser(line string) {
	m.pushBlock(Block{Kind: blockUser, Body: line})
}

func (m *bubbleModel) appendAssistant(text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	m.pushBlock(Block{Kind: blockAssistant, Body: text})
}

func (m *bubbleModel) appendAssistantDelta(text string) {
	if text == "" {
		return
	}
	if n := len(m.blocks); n > 0 && m.blocks[n-1].Kind == blockAssistant {
		m.blocks[n-1].Body += text
		m.trimBlocks()
		return
	}
	m.pushBlock(Block{Kind: blockAssistant, Body: text})
}

func (m *bubbleModel) appendError(text string) {
	m.pushBlock(Block{Kind: blockError, Body: text})
}

func (m *bubbleModel) appendMuted(text string) {
	m.pushBlock(Block{Kind: blockSystem, Body: text})
}

func (m *bubbleModel) appendToolRunning(name string) {
	m.pushBlock(Block{Kind: blockTool, Title: name, Running: true})
}

func (m *bubbleModel) applyToolResult(name string, result tool.Result, err error) {
	body := result.Output
	if result.CheckpointID != "" {
		body = joinBody(body, "checkpoint: "+result.CheckpointID)
	}
	if result.Truncated {
		body = joinBody(body, "output truncated")
	}
	if result.Denied {
		body = joinBody(body, "denied")
	}
	if result.ExitCode != nil {
		body = joinBody(body, fmt.Sprintf("exit %d", *result.ExitCode))
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || failureCode(result) == tool.ErrorCodeCanceled {
			block := Block{Kind: blockTool, Title: name, Body: "cancelled"}
			if m.replaceRunningTool(name, block) {
				return
			}
			m.pushBlock(block)
			return
		}
		errorBlock := Block{Kind: blockError, Title: name, Body: err.Error()}
		if result.Failure != nil {
			errorBlock.Body = fmt.Sprintf("[%s]: %s", result.Failure.Code, result.Failure.Message)
			errorBlock.Code = string(result.Failure.Code)
		}
		if m.replaceRunningTool(name, errorBlock) {
			return
		}
		m.pushBlock(errorBlock)
		return
	}
	block := Block{Kind: blockTool, Title: name, Body: body}
	if m.replaceRunningTool(name, block) {
		return
	}
	m.pushBlock(block)
}

func failureCode(result tool.Result) tool.ErrorCode {
	if result.Failure == nil {
		return ""
	}
	return result.Failure.Code
}

func (m *bubbleModel) replaceRunningTool(name string, replacement Block) bool {
	for index := len(m.blocks) - 1; index >= 0; index-- {
		block := m.blocks[index]
		if block.Kind != blockTool || block.Title != name || !block.Running {
			continue
		}
		m.blocks[index] = replacement
		return true
	}
	return false
}

func (m *bubbleModel) applyTurnEvent(event applicationturn.Event) {
	switch event.Kind {
	case applicationturn.EventTextDelta:
		m.appendAssistantDelta(event.Text)
	case applicationturn.EventToolCall:
		m.appendToolRunning(event.Call.Name)
	case applicationturn.EventToolResult:
		m.applyToolResult(event.Call.Name, event.Result, nil)
	case applicationturn.EventFailed:
		m.appendTurnFailure(event.Err)
	}
}

func (m *bubbleModel) appendTurnFailure(err error) {
	if err == nil {
		return
	}
	text := "turn failed: " + err.Error()
	kind := blockError
	if errors.Is(err, context.Canceled) {
		text = "turn cancelled"
		kind = blockSystem
	}
	if n := len(m.blocks); n > 0 && m.blocks[n-1].Kind == kind && m.blocks[n-1].Body == text {
		return
	}
	m.pushBlock(Block{Kind: kind, Body: text})
}

func (m *bubbleModel) appendTurnResult(
	events []applicationturn.Event,
	result applicationturn.Result,
	err error,
) {
	sawAssistant := false
	for _, event := range events {
		if event.Kind == applicationturn.EventTextDelta && event.Text != "" {
			sawAssistant = true
		}
		m.applyTurnEvent(event)
	}
	if !sawAssistant && result.Message.Content != "" {
		m.appendAssistant(result.Message.Content)
	}
	m.appendTurnFailure(err)
}

func (m *bubbleModel) appendToolResult(result tool.Result, err error) {
	name := result.ToolName
	if name == "" {
		for index := len(m.blocks) - 1; index >= 0; index-- {
			if m.blocks[index].Kind == blockTool && m.blocks[index].Running {
				name = m.blocks[index].Title
				break
			}
		}
	}
	m.applyToolResult(name, result, err)
	if err == nil {
		m.appendMuted("tool completed")
	}
}

func (m bubbleModel) renderBlocks() []string {
	lines := make([]string, 0)
	for _, block := range m.blocks {
		lines = append(lines, renderBlock(block)...)
	}
	return lines
}

func renderBlock(block Block) []string {
	body := strings.TrimRight(block.Body, "\n")
	switch block.Kind {
	case blockUser:
		return []string{userStyle.Render(glyphMark) + bodyStyle.Render(sanitizeBubbleText(body))}
	case blockAssistant:
		if body == "" {
			return nil
		}
		out := make([]string, 0)
		for i, line := range strings.Split(body, "\n") {
			prefix := "  "
			if i == 0 {
				prefix = ""
			}
			out = append(out, assistantStyle.Render(prefix+sanitizeBubbleText(line)))
		}
		return out
	case blockTool:
		header := glyphTool + block.Title
		if block.Running {
			header += " …"
		}
		out := []string{toolStyle.Render(header)}
		if body == "" {
			return out
		}
		for _, line := range strings.Split(body, "\n") {
			out = append(out, bodyStyle.Render("  "+sanitizeBubbleText(line)))
		}
		return out
	case blockError:
		text := body
		if block.Title != "" {
			text = block.Title + ": " + body
		}
		return []string{errorStyle.Render(sanitizeBubbleText(text))}
	default:
		if body == "" {
			return nil
		}
		out := make([]string, 0)
		for _, line := range strings.Split(body, "\n") {
			out = append(out, mutedStyle.Render(sanitizeBubbleText(line)))
		}
		return out
	}
}

func joinBody(existing string, extra string) string {
	if existing == "" {
		return extra
	}
	if extra == "" {
		return existing
	}
	return existing + "\n" + extra
}

func plainTranscript(model *bubbleModel) string {
	lines := make([]string, 0)
	for _, block := range model.blocks {
		if block.Title != "" {
			lines = append(lines, sanitizeBubbleText(block.Title))
		}
		if block.Body != "" {
			for _, line := range strings.Split(strings.TrimRight(block.Body, "\n"), "\n") {
				lines = append(lines, sanitizeBubbleText(line))
			}
		}
	}
	return strings.Join(lines, "\n")
}
