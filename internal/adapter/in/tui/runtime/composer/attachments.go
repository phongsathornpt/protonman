// Package composer owns the composer's local attachment state: the image
// references the user has staged in the prompt, their rendered chips, and the
// paste-path classification that decides whether a pasted path is an image.
//
// Attachment records stay TUI-local until they are snapshotted into a
// provider-neutral conversation input.
package composer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"github.com/charmbracelet/x/ansi"

	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

// image is a single staged image reference. Placeholder is the literal token
// embedded in the prompt text so the user can see and delete the attachment
// inline.
type image struct {
	placeholder string
	path        string
	temporary   bool
}

// State holds the composer's staged image attachments in prompt order.
type State struct {
	images []image
}

// NewState returns an empty attachment state.
func NewState() State {
	return State{}
}

// Len reports how many attachments are staged.
func (s *State) Len() int {
	if s == nil {
		return 0
	}
	return len(s.images)
}

// Attach stages a user-referenced image at path.
func (s *State) Attach(prompt *textarea.Model, path string) {
	s.attach(prompt, path, false)
}

// AttachTemporary stages an image whose file the TUI owns and may delete once
// the attachment is snapshotted, removed, or discarded.
func (s *State) AttachTemporary(prompt *textarea.Model, path string) {
	s.attach(prompt, path, true)
}

func (s *State) attach(prompt *textarea.Model, path string, temporary bool) {
	if s == nil || prompt == nil || strings.TrimSpace(path) == "" {
		return
	}
	placeholder := ImageLabel(len(s.images) + 1)
	value := prompt.Value()
	if value != "" && !strings.HasSuffix(value, " ") && !strings.HasSuffix(value, "\n") {
		value += " "
	}
	value += placeholder
	prompt.SetValue(value)
	prompt.CursorEnd()
	s.images = append(s.images, image{placeholder: placeholder, path: path, temporary: temporary})
}

// RemoveLast drops the most recent attachment and erases its placeholder from
// the prompt. It reports whether an attachment was removed.
func (s *State) RemoveLast(prompt *textarea.Model) bool {
	if s == nil || len(s.images) == 0 {
		return false
	}
	last := s.images[len(s.images)-1]
	if last.temporary && strings.TrimSpace(last.path) != "" {
		_ = os.Remove(last.path)
	}
	s.images = s.images[:len(s.images)-1]
	if prompt != nil {
		val := prompt.Value()
		val = strings.Replace(val, last.placeholder, "", 1)
		val = strings.TrimSpace(val)
		prompt.SetValue(val)
		prompt.CursorEnd()
	}
	return true
}

// Release detaches every attachment without deleting any file. Callers that
// hand ownership to a conversation queue use this.
func (s *State) Release() {
	if s == nil {
		return
	}
	clear(s.images)
	s.images = nil
}

// Clear detaches every attachment and deletes the temporary files the TUI owns.
func (s *State) Clear() {
	s.discard()
}

func (s *State) discard() {
	if s == nil {
		return
	}
	CleanupImages(s.images)
	s.Release()
}

// Snapshot converts staged attachments into conversation inputs. A non-nil
// prompt reconciles the placeholders against the current text first.
func (s *State) Snapshot(prompt *textarea.Model) []tuiconv.Attachment {
	if s == nil {
		return nil
	}
	if prompt != nil {
		s.syncWithText(prompt)
	}
	out := make([]tuiconv.Attachment, 0, len(s.images))
	for _, img := range s.images {
		out = append(out, tuiconv.Attachment{
			Placeholder: img.placeholder,
			Path:        img.path,
			Temporary:   img.temporary,
		})
	}
	return out
}

// syncWithText drops attachments whose placeholder the user deleted from the
// prompt and renumbers the survivors so labels stay contiguous.
func (s *State) syncWithText(prompt *textarea.Model) {
	if s == nil || prompt == nil || len(s.images) == 0 {
		return
	}
	text := prompt.Value()
	kept := make([]image, 0, len(s.images))
	for _, img := range s.images {
		if strings.Contains(text, img.placeholder) {
			kept = append(kept, img)
			continue
		}
		if img.temporary {
			_ = os.Remove(img.path)
		}
	}
	s.images = kept
	for i := range s.images {
		expected := ImageLabel(i + 1)
		if s.images[i].placeholder == expected {
			continue
		}
		text = strings.Replace(text, s.images[i].placeholder, expected, 1)
		s.images[i].placeholder = expected
	}
	if text != prompt.Value() {
		prompt.SetValue(text)
		prompt.CursorEnd()
	}
}

// RenderChips renders the attachment strip, truncated to width when positive.
func (s *State) RenderChips(width int, icons tuistyle.IconSet) string {
	if s == nil || len(s.images) == 0 {
		return ""
	}
	icons = tuistyle.OrUnicodeIcons(icons)
	chips := make([]string, 0, len(s.images))
	for _, img := range s.images {
		name := filepath.Base(img.path)
		meta := ""
		if fi, err := os.Stat(img.path); err == nil && fi.Size() > 0 {
			sz := fi.Size()
			switch {
			case sz >= 1024*1024:
				meta = fmt.Sprintf("%.1f MB", float64(sz)/(1024*1024))
			case sz >= 1024:
				meta = fmt.Sprintf("%d KB", sz/1024)
			default:
				meta = fmt.Sprintf("%d B", sz)
			}
		}
		label := name
		if meta != "" {
			label = fmt.Sprintf("%s · %s", name, meta)
		}
		pillText := fmt.Sprintf("%s%s", icons.Image, label)
		chips = append(chips, tuistyle.ComposerAttachmentPill.Render(pillText)+" "+tuistyle.ComposerAttachmentRemove.Render("✕"))
	}
	joined := strings.Join(chips, "  ")
	if width > 0 && ansi.StringWidth(joined) > width {
		joined = textview.TruncateEllipsis(joined, width)
	}
	return joined
}

// ImageLabel returns the prompt-visible placeholder for the nth attachment.
func ImageLabel(index int) string {
	return fmt.Sprintf("[Image #%d]", index)
}

// CleanupImages deletes the temporary files owned by images.
func CleanupImages(images []image) {
	for _, img := range images {
		if img.temporary && strings.TrimSpace(img.path) != "" {
			_ = os.Remove(img.path)
		}
	}
}

// CleanupQueuedAttachments deletes the temporary files still owned by a queued
// submission that was discarded before it ran.
func CleanupQueuedAttachments(attachments []tuiconv.Attachment) {
	for _, attachment := range attachments {
		if attachment.Temporary && strings.TrimSpace(attachment.Path) != "" {
			_ = os.Remove(attachment.Path)
		}
	}
}

// StripPlaceholders removes attachment placeholders from submitted text.
func StripPlaceholders(text string, attachments []tuiconv.Attachment) string {
	for _, attachment := range attachments {
		if attachment.Placeholder != "" {
			text = strings.ReplaceAll(text, attachment.Placeholder, "")
		}
	}
	return strings.TrimSpace(text)
}
