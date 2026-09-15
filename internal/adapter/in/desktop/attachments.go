//go:build desktop

package desktop

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
)

const maxPromptAttachmentBytes = 20 << 20

type composerAttachment struct {
	Name     string
	URI      string
	MIMEType string
	Data     []byte
	Image    bool
}

func (a *application) openAttachmentPicker() {
	if a.window == nil {
		return
	}
	picker := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			a.setStatus("Attach failed · " + err.Error())
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()
		attachment, err := a.readPromptAttachment(reader)
		if err != nil {
			a.setStatus("Attach failed · " + err.Error())
			return
		}
		a.mu.Lock()
		a.attachments = append(a.attachments, attachment)
		a.mu.Unlock()
		a.renderAttachments()
	}, a.window)
	picker.Show()
}

func (a *application) readPromptAttachment(reader fyne.URIReadCloser) (composerAttachment, error) {
	uri := reader.URI()
	name := uri.Name()
	mimeType := strings.TrimSpace(mime.TypeByExtension(strings.ToLower(filepath.Ext(name))))
	if cut := strings.IndexByte(mimeType, ';'); cut >= 0 {
		mimeType = strings.TrimSpace(mimeType[:cut])
	}
	image := strings.HasPrefix(mimeType, "image/")
	textResource := strings.HasPrefix(mimeType, "text/") || mimeType == "application/json" || mimeType == "application/xml"

	a.mu.Lock()
	caps := a.agentCapabilities.PromptCapabilities
	a.mu.Unlock()
	if image && !caps.Image {
		return composerAttachment{}, errors.New("connected ACP agent does not advertise image prompts")
	}
	if !image && !caps.EmbeddedContext {
		return composerAttachment{}, errors.New("connected ACP agent does not advertise embedded context")
	}
	if !image && mimeType != "" && !textResource {
		return composerAttachment{}, fmt.Errorf("%s is not a supported text resource", name)
	}

	limited := io.LimitReader(reader, maxPromptAttachmentBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return composerAttachment{}, err
	}
	if len(data) > maxPromptAttachmentBytes {
		return composerAttachment{}, fmt.Errorf("%s exceeds the 20 MiB attachment limit", name)
	}
	if mimeType == "" {
		mimeType = "text/plain"
	}
	return composerAttachment{Name: name, URI: uri.String(), MIMEType: mimeType, Data: data, Image: image}, nil
}

func (a *application) promptContentBlocks(text string, attachments []composerAttachment) []acpclient.ContentBlock {
	blocks := make([]acpclient.ContentBlock, 0, len(attachments)+1)
	if text = strings.TrimSpace(text); text != "" {
		blocks = append(blocks, acpclient.ContentBlock{Type: "text", Text: text})
	}
	for _, attachment := range attachments {
		if attachment.Image {
			blocks = append(blocks, acpclient.ContentBlock{
				Type: "image", MIMEType: attachment.MIMEType,
				Data: base64.StdEncoding.EncodeToString(attachment.Data),
			})
			continue
		}
		blocks = append(blocks, acpclient.ContentBlock{
			Type: "resource",
			Resource: &acpclient.EmbeddedTextResource{
				URI: attachment.URI, MIMEType: attachment.MIMEType, Text: string(attachment.Data),
			},
		})
	}
	return blocks
}

func (a *application) removeAttachment(index int) {
	a.mu.Lock()
	if index >= 0 && index < len(a.attachments) {
		a.attachments = append(a.attachments[:index:index], a.attachments[index+1:]...)
	}
	a.mu.Unlock()
	a.renderAttachments()
}

func (a *application) renderAttachments() {
	if a.attachmentStrip == nil {
		return
	}
	a.mu.Lock()
	items := append([]composerAttachment(nil), a.attachments...)
	caps := a.agentCapabilities.PromptCapabilities
	connected := a.client != nil
	a.mu.Unlock()
	fyne.Do(func() {
		a.attachmentStrip.Objects = nil
		for i, attachment := range items {
			i := i
			label := attachment.Name
			if attachment.Image {
				label = "Image · " + label
			}
			a.attachmentStrip.Add(widget.NewButton(label+" ×", func() { a.removeAttachment(i) }))
		}
		a.attachmentStrip.Refresh()
		if a.attachButton != nil {
			if connected && (caps.Image || caps.EmbeddedContext) {
				a.attachButton.Enable()
			} else {
				a.attachButton.Disable()
			}
		}
	})
}

func attachmentContainer(strip *fyne.Container) fyne.CanvasObject {
	return container.NewHScroll(strip)
}
