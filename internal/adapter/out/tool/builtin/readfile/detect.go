package readfile

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type artifactKind string

const (
	artifactText   artifactKind = "text"
	artifactJSON   artifactKind = "json"
	artifactJSONL  artifactKind = "jsonl"
	artifactCSV    artifactKind = "csv"
	artifactTSV    artifactKind = "tsv"
	artifactImage  artifactKind = "image"
	artifactBinary artifactKind = "binary"
)

type artifactInfo struct {
	Kind     artifactKind
	MIMEType string
}

func detectArtifact(file *os.File, path string) (artifactInfo, error) {
	header := make([]byte, 512)
	n, err := file.ReadAt(header, 0)
	if err != nil && err != io.EOF {
		return artifactInfo{}, err
	}
	header = header[:n]
	mimeType := http.DetectContentType(header)
	ext := strings.ToLower(filepath.Ext(path))

	switch {
	case mimeType == "image/png" || mimeType == "image/jpeg" || mimeType == "image/gif":
		return artifactInfo{Kind: artifactImage, MIMEType: mimeType}, nil
	case ext == ".json" || mimeType == "application/json":
		return artifactInfo{Kind: artifactJSON, MIMEType: "application/json"}, nil
	case ext == ".jsonl" || ext == ".ndjson":
		return artifactInfo{Kind: artifactJSONL, MIMEType: "application/x-ndjson"}, nil
	case ext == ".csv":
		return artifactInfo{Kind: artifactCSV, MIMEType: "text/csv"}, nil
	case ext == ".tsv":
		return artifactInfo{Kind: artifactTSV, MIMEType: "text/tab-separated-values"}, nil
	}
	if len(header) == 0 {
		return artifactInfo{Kind: artifactText, MIMEType: "text/plain; charset=utf-8"}, nil
	}
	if utf8.Valid(header) && !bytes.ContainsRune(header, '\x00') {
		if strings.HasPrefix(mimeType, "text/") {
			return artifactInfo{Kind: artifactText, MIMEType: mimeType}, nil
		}
		return artifactInfo{Kind: artifactText, MIMEType: "text/plain; charset=utf-8"}, nil
	}
	return artifactInfo{Kind: artifactBinary, MIMEType: mimeType}, nil
}

func (kind artifactKind) structured() bool {
	switch kind {
	case artifactJSON, artifactJSONL, artifactCSV, artifactTSV:
		return true
	default:
		return false
	}
}
