package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeArchive(t *testing.T, path string, names []string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	writer := tar.NewWriter(gz)
	for _, name := range names {
		body := []byte("binary")
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyChecksumRoundTrip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "protonman_1.2.3_linux_amd64.tar.gz")
	writeArchive(t, archive, []string{"protonman"})
	sum := sha256.Sum256(mustRead(t, archive))
	checksums := filepath.Join(dir, "checksums.txt")
	content := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), "protonman_1.2.3_linux_amd64.tar.gz")
	if err := os.WriteFile(checksums, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyChecksum(archive, checksums, "protonman_1.2.3_linux_amd64.tar.gz"); err != nil {
		t.Fatalf("VerifyChecksum() error = %v", err)
	}
	if err := os.WriteFile(checksums, []byte("deadbeef  other.tar.gz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyChecksum(archive, checksums, "protonman_1.2.3_linux_amd64.tar.gz"); err == nil {
		t.Fatal("VerifyChecksum() expected missing entry error")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExtractSingleBinaryGuards(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	writeArchive(t, archive, []string{"protonman", "extra"})
	if _, err := ExtractSingleBinary(archive, dir); err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Fatalf("ExtractSingleBinary multi-entry error = %v", err)
	}
	writeArchive(t, archive, []string{"wrong"})
	if _, err := ExtractSingleBinary(archive, dir); err == nil || !strings.Contains(err.Error(), "unexpected archive") {
		t.Fatalf("ExtractSingleBinary name error = %v", err)
	}
	writeArchive(t, archive, []string{"protonman"})
	staged, err := ExtractSingleBinary(archive, dir)
	if err != nil {
		t.Fatalf("ExtractSingleBinary() error = %v", err)
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("staged binary missing: %v", err)
	}
}

func TestResolveLatestTagRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://github.com/phongsathornpt/protonman/releases/tag/v9.9.9")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	_ = server
	client := newUpdateClient()
	tag, err := latestTagFromRedirect(t.Context(), replaceHostClient(t, client, server, "https://github.com/phongsathornpt/protonman/releases/latest"))
	if err != nil {
		t.Fatalf("latestTagFromRedirect() error = %v", err)
	}
	if tag != "v9.9.9" {
		t.Fatalf("tag = %q, want v9.9.9", tag)
	}
}

func replaceHostClient(t *testing.T, client *http.Client, server *httptest.Server, _ string) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			proxied, err := http.NewRequestWithContext(r.Context(), r.Method, server.URL, nil)
			if err != nil {
				return nil, err
			}
			return http.DefaultTransport.RoundTrip(proxied)
		}),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSupportedPlatform(t *testing.T) {
	if err := SupportedPlatform("linux", "amd64"); err != nil {
		t.Fatalf("SupportedPlatform linux/amd64 error = %v", err)
	}
	if err := SupportedPlatform("windows", "amd64"); err == nil {
		t.Fatal("SupportedPlatform windows/amd64 expected error")
	}
}
