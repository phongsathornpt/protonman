package sessionfs

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/session"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const maxSessionAttachmentBytes = 32 * 1024 * 1024

func externalizeImageParts(resources session.Resources, state State) (State, error) {
	needsDirectory := false
	for messageIndex := range state.Messages {
		for partIndex := range state.Messages[messageIndex].Parts {
			part := &state.Messages[messageIndex].Parts[partIndex]
			if part.Type != sdk.ContentPartImage {
				continue
			}
			if strings.TrimSpace(part.Data) == "" {
				if strings.TrimSpace(part.Blob) == "" {
					return State{}, fmt.Errorf("session image part has neither data nor blob reference")
				}
				continue
			}
			needsDirectory = true
			raw, err := base64.StdEncoding.DecodeString(part.Data)
			if err != nil {
				return State{}, fmt.Errorf("decode session image part: %w", err)
			}
			if len(raw) > maxSessionAttachmentBytes {
				return State{}, fmt.Errorf("session image part exceeds %d byte limit", maxSessionAttachmentBytes)
			}
			name := attachmentBlobName(raw)
			if err := writeAttachmentBlob(resources.Attachments, name, raw); err != nil {
				return State{}, err
			}
			part.Blob = name
			part.Data = ""
		}
	}
	if needsDirectory {
		if err := os.Chmod(resources.Attachments, 0o700); err != nil {
			return State{}, fmt.Errorf("protect session attachment directory: %w", err)
		}
	}
	return state, nil
}

func hydrateImageParts(resources session.Resources, state State) (State, error) {
	for messageIndex := range state.Messages {
		for partIndex := range state.Messages[messageIndex].Parts {
			part := &state.Messages[messageIndex].Parts[partIndex]
			if part.Type != sdk.ContentPartImage || strings.TrimSpace(part.Data) != "" {
				continue
			}
			name := strings.TrimSpace(part.Blob)
			if err := validateAttachmentBlobName(name); err != nil {
				return State{}, err
			}
			path := filepath.Join(resources.Attachments, name)
			file, err := os.Open(path)
			if err != nil {
				return State{}, fmt.Errorf("open session attachment %q: %w", name, err)
			}
			raw, readErr := io.ReadAll(io.LimitReader(file, maxSessionAttachmentBytes+1))
			closeErr := file.Close()
			if readErr != nil {
				return State{}, fmt.Errorf("read session attachment %q: %w", name, readErr)
			}
			if closeErr != nil {
				return State{}, fmt.Errorf("close session attachment %q: %w", name, closeErr)
			}
			if len(raw) > maxSessionAttachmentBytes {
				return State{}, fmt.Errorf("session attachment %q exceeds %d byte limit", name, maxSessionAttachmentBytes)
			}
			if attachmentBlobName(raw) != name {
				return State{}, fmt.Errorf("session attachment %q failed digest verification", name)
			}
			part.Data = base64.StdEncoding.EncodeToString(raw)
		}
	}
	return state, nil
}

func attachmentBlobName(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]) + ".img"
}

func validateAttachmentBlobName(name string) error {
	if name == "" || filepath.Base(name) != name || !strings.HasSuffix(name, ".img") {
		return fmt.Errorf("invalid session attachment reference %q", name)
	}
	hexDigest := strings.TrimSuffix(name, ".img")
	if len(hexDigest) != sha256.Size*2 {
		return fmt.Errorf("invalid session attachment reference %q", name)
	}
	if _, err := hex.DecodeString(hexDigest); err != nil {
		return fmt.Errorf("invalid session attachment reference %q: %w", name, err)
	}
	return nil
}

func writeAttachmentBlob(root, name string, raw []byte) (writeErr error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create session attachment directory: %w", err)
	}
	path := filepath.Join(root, name)
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() == int64(len(raw)) {
		return nil
	}
	file, err := os.CreateTemp(root, ".attachment-*.tmp")
	if err != nil {
		return fmt.Errorf("create session attachment temp file: %w", err)
	}
	temporaryPath := file.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); closeErr != nil && writeErr == nil {
				writeErr = closeErr
			}
		}
		_ = os.Remove(temporaryPath)
	}()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect session attachment temp file: %w", err)
	}
	if _, err := file.Write(raw); err != nil {
		return fmt.Errorf("write session attachment: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync session attachment: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close session attachment: %w", err)
	}
	closed = true
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install session attachment: %w", err)
	}
	return nil
}
