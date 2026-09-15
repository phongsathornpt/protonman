package imageprep

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotLocalKeepsCanonicalRawBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.png")
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := SnapshotLocal(path)
	if err != nil {
		t.Fatalf("SnapshotLocal() error = %v", err)
	}
	if snapshot.MIMEType != "image/png" || snapshot.Width != 2 || snapshot.Height != 3 {
		t.Fatalf("snapshot metadata = %+v", snapshot)
	}
	if !bytes.Equal(snapshot.Bytes, encoded.Bytes()) {
		t.Fatal("snapshot payload differs from source bytes")
	}
	if snapshot.Digest != sha256.Sum256(encoded.Bytes()) {
		t.Fatalf("snapshot digest = %x", snapshot.Digest)
	}

	clone := snapshot.CloneBytes()
	clone[0] ^= 0xff
	if bytes.Equal(clone, snapshot.Bytes) {
		t.Fatal("CloneBytes() shares backing storage with snapshot")
	}
}
