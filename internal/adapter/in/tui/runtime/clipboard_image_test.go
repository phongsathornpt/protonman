//go:build darwin || linux

package runtime

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func testClipboardPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestWriteClipboardTempPNGUsesPrivateFile(t *testing.T) {
	path, err := writeClipboardTempPNG(testClipboardPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("clipboard temp mode = %o, want 600", got)
	}
}

func TestClipboardImageShortcutStartsAsyncRead(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	updated, command := m.Update(testCtrl('v'))
	m = updated.(*bubbleModel)
	if command == nil {
		t.Fatal("ctrl+v did not start clipboard image read")
	}
	if got := len(m.panes.bottom.composer.attachments.localImages); got != 0 {
		t.Fatalf("attachments before async result = %d, want 0", got)
	}
}

func TestClipboardImageResultAttachesToMatchingDraftAndResetCleansTemp(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.panes.bottom.prompt().SetValue("inspect")
	path, err := writeClipboardTempPNG(testClipboardPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	m.updateClipboardImageLoaded(clipboardImageLoadedMsg{
		draftText: "inspect", path: path, width: 2, height: 2,
	})
	images := m.panes.bottom.composer.attachments.localImages
	if len(images) != 1 || images[0].path != path || !images[0].temporary {
		t.Fatalf("clipboard attachment = %+v", images)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("owned clipboard temp missing before reset: %v", err)
	}
	m.resetPrompt()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("clipboard temp survived discarded draft: %v", err)
	}
}

func TestClipboardImageStaleDraftDeletesTemp(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.panes.bottom.prompt().SetValue("new draft")
	path, err := writeClipboardTempPNG(testClipboardPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	m.updateClipboardImageLoaded(clipboardImageLoadedMsg{draftText: "old draft", path: path})
	if got := len(m.panes.bottom.composer.attachments.localImages); got != 0 {
		t.Fatalf("stale clipboard result attached %d images", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale clipboard temp survived: %v", err)
	}
}

func TestClipboardTempOwnershipTransfersIntoQueue(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.busy = true
	prompt := m.panes.bottom.prompt()
	prompt.SetValue("queue this")
	path, err := writeClipboardTempPNG(testClipboardPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	m.panes.bottom.composer.attachments.attachTemporaryImage(prompt, path)
	_ = m.submit()

	queued := m.conversation.QueuedInputs()
	if len(queued) != 1 || len(queued[0].Attachments) != 1 || !queued[0].Attachments[0].Temporary {
		t.Fatalf("queued input = %+v", queued)
	}
	if got := len(m.panes.bottom.composer.attachments.localImages); got != 0 {
		t.Fatalf("composer retained %d transferred attachments", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("transferred temp removed before queue finished: %v", err)
	}
	m.clearQueuedInputs()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("queued clipboard temp survived queue clear: %v", err)
	}
}
