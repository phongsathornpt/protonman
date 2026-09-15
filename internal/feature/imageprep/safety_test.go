package imageprep

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"strings"
	"testing"
)

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func pngHeaderOnly(width, height uint32) []byte {
	var out bytes.Buffer
	out.Write(pngSignature)
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 6 // RGBA
	writePNGChunk(&out, "IHDR", ihdr)
	writePNGChunk(&out, "IDAT", nil)
	writePNGChunk(&out, "IEND", nil)
	return out.Bytes()
}

func writePNGChunk(out *bytes.Buffer, kind string, data []byte) {
	_ = binary.Write(out, binary.BigEndian, uint32(len(data)))
	out.WriteString(kind)
	out.Write(data)
	crcInput := append([]byte(kind), data...)
	_ = binary.Write(out, binary.BigEndian, crc32.ChecksumIEEE(crcInput))
}

func TestPrepareRejectsOversizedDimensionsBeforeFullDecode(t *testing.T) {
	raw := pngHeaderOnly(uint32(MaxSourceDimension+1), 1)
	_, err := Prepare("image/png", base64.StdEncoding.EncodeToString(raw), OriginalPolicy())
	if err == nil || !strings.Contains(err.Error(), "safe decode limit") {
		t.Fatalf("Prepare() error = %v, want safe decode limit", err)
	}
}

func TestPrepareRejectsOversizedPixelAreaBeforeFullDecode(t *testing.T) {
	// Individually reasonable dimensions with a total area beyond the hard cap.
	raw := pngHeaderOnly(4096, 4096)
	_, err := Prepare("image/png", base64.StdEncoding.EncodeToString(raw), OriginalPolicy())
	if err == nil || !strings.Contains(err.Error(), "safe decode limit") {
		t.Fatalf("Prepare() error = %v, want safe decode limit", err)
	}
}

func TestPrepareDoesNotPassthroughTruncatedImageAfterMetadataValidation(t *testing.T) {
	raw := pngHeaderOnly(64, 64)
	_, err := Prepare("image/png", base64.StdEncoding.EncodeToString(raw), DefaultPolicy())
	if err == nil || !strings.Contains(err.Error(), "decode image") {
		t.Fatalf("Prepare() error = %v, want full image validation failure", err)
	}
}
