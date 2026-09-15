package runtime

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/feature/imageprep"
	"golang.design/x/clipboard"
)

type clipboardImageLoadedMsg struct {
	draftText        string
	draftAttachments int
	path             string
	width            int
	height           int
	err              error
}

func loadClipboardImage(draftText string, draftAttachments int) tea.Cmd {
	return func() tea.Msg {
		result := clipboardImageLoadedMsg{draftText: draftText, draftAttachments: draftAttachments}
		if err := clipboard.Init(); err != nil {
			result.err = fmt.Errorf("clipboard unavailable: %w", err)
			return result
		}
		raw := clipboard.Read(clipboard.FmtImage)
		if len(raw) == 0 {
			result.err = fmt.Errorf("clipboard does not contain an image")
			return result
		}
		if len(raw) > imageprep.MaxSnapshotBytes {
			result.err = fmt.Errorf("clipboard image exceeds %d byte limit", imageprep.MaxSnapshotBytes)
			return result
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil {
			result.err = fmt.Errorf("decode clipboard image: %w", err)
			return result
		}
		if format != "png" {
			result.err = fmt.Errorf("clipboard image format %q is not PNG", format)
			return result
		}
		if err := imageprep.ValidateSourceDimensions(config.Width, config.Height); err != nil {
			result.err = fmt.Errorf("clipboard image: %w", err)
			return result
		}
		path, err := writeClipboardTempPNG(raw)
		if err != nil {
			result.err = err
			return result
		}
		result.path = path
		result.width = config.Width
		result.height = config.Height
		return result
	}
}

func writeClipboardTempPNG(raw []byte) (path string, writeErr error) {
	file, err := os.CreateTemp("", "protonman-clipboard-*.png")
	if err != nil {
		return "", fmt.Errorf("create clipboard image temp file: %w", err)
	}
	path = file.Name()
	closed := false
	defer func() {
		if !closed {
			if err := file.Close(); err != nil && writeErr == nil {
				writeErr = fmt.Errorf("close clipboard image temp file: %w", err)
			}
		}
		if writeErr != nil {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", fmt.Errorf("protect clipboard image temp file: %w", err)
	}
	if _, err := file.Write(raw); err != nil {
		return "", fmt.Errorf("write clipboard image temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close clipboard image temp file: %w", err)
	}
	closed = true
	return path, nil
}

func (m *bubbleModel) beginClipboardImagePaste() tea.Cmd {
	if m == nil || m.panes.bottom == nil || !m.panes.bottom.composerVisible() || m.panes.bottom.prompt() == nil {
		return nil
	}
	if !m.currentModelAcceptsImageInput() {
		m.appendError(m.imageInputsNotSupportedMessage())
		m.requestRelayout()
		return nil
	}
	prompt := m.panes.bottom.prompt()
	return loadClipboardImage(prompt.Value(), len(m.panes.bottom.composer.attachments.localImages))
}

func (m *bubbleModel) updateClipboardImageLoaded(message clipboardImageLoadedMsg) tea.Cmd {
	if m == nil || m.panes.bottom == nil || !m.panes.bottom.composerVisible() || m.panes.bottom.prompt() == nil {
		if message.path != "" {
			_ = os.Remove(message.path)
		}
		return nil
	}
	prompt := m.panes.bottom.prompt()
	if prompt.Value() != message.draftText || len(m.panes.bottom.composer.attachments.localImages) != message.draftAttachments {
		if message.path != "" {
			_ = os.Remove(message.path)
		}
		return nil
	}
	if message.err != nil {
		m.appendError(message.err.Error())
		m.requestRelayout()
		return nil
	}
	if !m.currentModelAcceptsImageInput() {
		_ = os.Remove(message.path)
		m.appendError(m.imageInputsNotSupportedMessage())
		m.requestRelayout()
		return nil
	}
	m.panes.bottom.composer.attachments.attachTemporaryImage(prompt, message.path)
	m.syncSlashView()
	m.requestRelayout()
	return nil
}
