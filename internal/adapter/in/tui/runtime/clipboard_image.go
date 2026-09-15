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
	requestID uint64
	path      string
	width     int
	height    int
	err       error
}

func loadClipboardImage(requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.Init(); err != nil {
			return clipboardImageLoadedMsg{requestID: requestID, err: fmt.Errorf("clipboard unavailable: %w", err)}
		}
		raw := clipboard.Read(clipboard.FmtImage)
		if len(raw) == 0 {
			return clipboardImageLoadedMsg{requestID: requestID, err: fmt.Errorf("clipboard does not contain an image")}
		}
		if len(raw) > imageprep.MaxSnapshotBytes {
			return clipboardImageLoadedMsg{requestID: requestID, err: fmt.Errorf("clipboard image exceeds %d byte limit", imageprep.MaxSnapshotBytes)}
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil {
			return clipboardImageLoadedMsg{requestID: requestID, err: fmt.Errorf("decode clipboard image: %w", err)}
		}
		if format != "png" {
			return clipboardImageLoadedMsg{requestID: requestID, err: fmt.Errorf("clipboard image format %q is not PNG", format)}
		}
		if err := imageprep.ValidateSourceDimensions(config.Width, config.Height); err != nil {
			return clipboardImageLoadedMsg{requestID: requestID, err: fmt.Errorf("clipboard image: %w", err)}
		}
		path, err := writeClipboardTempPNG(raw)
		if err != nil {
			return clipboardImageLoadedMsg{requestID: requestID, err: err}
		}
		return clipboardImageLoadedMsg{requestID: requestID, path: path, width: config.Width, height: config.Height}
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
	if m == nil || m.clipboardImageLoading {
		return nil
	}
	if !m.currentModelAcceptsImageInput() {
		m.appendError(m.imageInputsNotSupportedMessage())
		m.requestRelayout()
		return nil
	}
	m.clipboardImageRequestID++
	requestID := m.clipboardImageRequestID
	m.clipboardImageLoading = true
	m.requestRelayout()
	return loadClipboardImage(requestID)
}

func (m *bubbleModel) updateClipboardImageLoaded(message clipboardImageLoadedMsg) tea.Cmd {
	if message.requestID != m.clipboardImageRequestID {
		if message.path != "" {
			_ = os.Remove(message.path)
		}
		return nil
	}
	m.clipboardImageLoading = false
	if message.err != nil {
		m.appendError(message.err.Error())
		m.requestRelayout()
		return nil
	}
	if m.panes.bottom == nil || !m.panes.bottom.composerVisible() || m.panes.bottom.prompt() == nil {
		_ = os.Remove(message.path)
		return nil
	}
	if !m.currentModelAcceptsImageInput() {
		_ = os.Remove(message.path)
		m.appendError(m.imageInputsNotSupportedMessage())
		m.requestRelayout()
		return nil
	}
	prompt := m.panes.bottom.prompt()
	m.panes.bottom.composer.attachments.attachTemporaryImage(prompt, message.path)
	m.syncSlashView()
	m.requestRelayout()
	return nil
}

func (m *bubbleModel) cancelClipboardImagePaste() bool {
	if m == nil || !m.clipboardImageLoading {
		return false
	}
	m.clipboardImageRequestID++
	m.clipboardImageLoading = false
	m.requestRelayout()
	return true
}
