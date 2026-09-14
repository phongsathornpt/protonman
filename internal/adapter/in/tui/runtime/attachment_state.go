package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
)

type localImageAttachment struct {
	placeholder string
	path        string
}

type attachmentState struct {
	localImages []localImageAttachment
}

func localImageLabel(index int) string {
	return fmt.Sprintf("[Image #%d]", index)
}

func (s *attachmentState) attachImage(prompt *textarea.Model, path string) {
	if s == nil || prompt == nil || strings.TrimSpace(path) == "" {
		return
	}
	placeholder := localImageLabel(len(s.localImages) + 1)
	value := prompt.Value()
	if value != "" && !strings.HasSuffix(value, " ") && !strings.HasSuffix(value, "\n") {
		value += " "
	}
	value += placeholder
	prompt.SetValue(value)
	prompt.CursorEnd()
	s.localImages = append(s.localImages, localImageAttachment{placeholder: placeholder, path: path})
}

func (s *attachmentState) clear() {
	if s == nil {
		return
	}
	clear(s.localImages)
	s.localImages = nil
}

func (s *attachmentState) syncWithText(prompt *textarea.Model) {
	if s == nil || prompt == nil || len(s.localImages) == 0 {
		return
	}
	text := prompt.Value()
	kept := make([]localImageAttachment, 0, len(s.localImages))
	for _, image := range s.localImages {
		if strings.Contains(text, image.placeholder) {
			kept = append(kept, image)
		}
	}
	s.localImages = kept
	for i := range s.localImages {
		expected := localImageLabel(i + 1)
		if s.localImages[i].placeholder == expected {
			continue
		}
		text = strings.Replace(text, s.localImages[i].placeholder, expected, 1)
		s.localImages[i].placeholder = expected
	}
	if text != prompt.Value() {
		prompt.SetValue(text)
		prompt.CursorEnd()
	}
}

func (s *attachmentState) snapshot(prompt *textarea.Model) []tuiconv.Attachment {
	if s == nil {
		return nil
	}
	if prompt != nil {
		s.syncWithText(prompt)
	}
	out := make([]tuiconv.Attachment, 0, len(s.localImages))
	for _, image := range s.localImages {
		out = append(out, tuiconv.Attachment{Placeholder: image.placeholder, Path: image.path})
	}
	return out
}

func stripAttachmentPlaceholders(text string, attachments []tuiconv.Attachment) string {
	for _, attachment := range attachments {
		if attachment.Placeholder != "" {
			text = strings.ReplaceAll(text, attachment.Placeholder, "")
		}
	}
	return strings.TrimSpace(text)
}

func submissionDisplayText(input tuiconv.QueuedInput) string {
	parts := make([]string, 0, len(input.Attachments)+1)
	if strings.TrimSpace(input.Text) != "" {
		parts = append(parts, strings.TrimSpace(input.Text))
	}
	for i, attachment := range input.Attachments {
		label := attachment.Placeholder
		if strings.TrimSpace(label) == "" {
			label = localImageLabel(i + 1)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " ")
}

func localImagePathFromPaste(content, workDir string) (string, bool) {
	if strings.TrimSpace(content) == "" || strings.ContainsAny(content, "\r\n") {
		return "", false
	}
	cleaned := normalizePastedPath(content, workDir)
	candidate := strings.TrimSpace(cleaned)
	if candidate == "" {
		return "", false
	}
	path := candidate
	if !filepath.IsAbs(path) && strings.TrimSpace(workDir) != "" {
		path = filepath.Join(workDir, path)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return filepath.Clean(path), true
	default:
		return "", false
	}
}

func (m *bubbleModel) currentModelAcceptsImageInput() bool {
	if m == nil || strings.TrimSpace(m.activeModel) == "" {
		return true
	}
	var remote *model.RemoteModel
	if candidate, ok := m.activeRemoteModel(); ok {
		remote = &candidate
	}
	profile := model.ResolveModelProfile(m.activeProvider, m.activeModel, remote)
	return profile.Capabilities.Vision != modelprofile.SupportNo
}

func (m *bubbleModel) imageInputsNotSupportedMessage() string {
	modelID := strings.TrimSpace(m.activeModel)
	if modelID == "" {
		modelID = "current model"
	}
	return fmt.Sprintf("model %s does not support image inputs; remove images or switch models", modelID)
}
