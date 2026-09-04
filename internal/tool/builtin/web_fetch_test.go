package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/sandbox"
)

func TestWebFetchHonorsBlockedNetworkPolicy(t *testing.T) {
	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkBlocked})
	_, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-1", "web_fetch", map[string]any{
		"url": "https://example.com",
	}))
	if !errors.Is(err, sandbox.ErrNetworkDenied) {
		t.Fatalf("Execute() error = %v, want network denied", err)
	}
}

func TestWebFetchReadsAllowedLocalServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("ok-body"))
	}))
	t.Cleanup(server.Close)

	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted, AllowLocalhost: true})
	result, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-2", "web_fetch", map[string]any{
		"url": server.URL,
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "ok-body" {
		t.Fatalf("output = %q, want ok-body", result.Output)
	}
}

func TestWebFetchDefinitionKind(t *testing.T) {
	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted})
	if handler.Definition().Kind != "web_fetch" {
		t.Fatalf("kind = %s", handler.Definition().Kind)
	}
	if _, err := json.Marshal(handler.Definition().InputSchema); err != nil {
		t.Fatalf("schema marshal error = %v", err)
	}
}

func TestWebFetchRejectsInvalidSchemes(t *testing.T) {
	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted})
	invalidURLs := []string{
		"file:///etc/passwd",
		"ftp://example.com/file",
		"example.com/api",
		"://malformed",
		"",
	}
	for _, u := range invalidURLs {
		t.Run(u, func(t *testing.T) {
			_, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-scheme", "web_fetch", map[string]any{
				"url": u,
			}))
			if err == nil {
				t.Fatalf("expected error for URL %q, got nil", u)
			}
		})
	}
}

func TestWebFetchHaltsRedirectLoops(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		http.Redirect(writer, req, server.URL+"/loop", http.StatusFound)
	}))
	t.Cleanup(server.Close)

	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted, AllowLocalhost: true})
	_, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-loop", "web_fetch", map[string]any{
		"url": server.URL,
	}))
	if err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Fatalf("expected redirect loop error, got: %v", err)
	}
}

func TestWebFetchSendsDefaultHeaders(t *testing.T) {
	var receivedUA, receivedAccept string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		receivedUA = req.Header.Get("User-Agent")
		receivedAccept = req.Header.Get("Accept")
		_, _ = writer.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted, AllowLocalhost: true})
	_, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-headers", "web_fetch", map[string]any{
		"url": server.URL,
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasPrefix(receivedUA, "Proton/1.0") {
		t.Errorf("expected User-Agent starting with Proton/1.0, got: %q", receivedUA)
	}
	if !strings.Contains(receivedAccept, "text/html") {
		t.Errorf("expected Accept containing text/html, got: %q", receivedAccept)
	}
}

func TestWebFetchTruncationNotice(t *testing.T) {
	hugeBody := strings.Repeat("A", 300*1024)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		_, _ = writer.Write([]byte(hugeBody))
	}))
	t.Cleanup(server.Close)

	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted, AllowLocalhost: true})
	result, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-trunc", "web_fetch", map[string]any{
		"url": server.URL,
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.Truncated {
		t.Errorf("expected Truncated = true")
	}
	if !strings.HasSuffix(result.Output, "\n[output truncated at 256 KiB]") {
		t.Errorf("expected output to end with truncation notice, got length %d", len(result.Output))
	}
}

func TestWebFetchStatusErrorText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "resource missing", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted, AllowLocalhost: true})
	result, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-404", "web_fetch", map[string]any{
		"url": server.URL,
	}))
	if err == nil || !strings.Contains(err.Error(), "web_fetch status 404: Not Found") {
		t.Fatalf("expected status 404: Not Found error, got: %v", err)
	}
	if !strings.Contains(result.Output, "resource missing") {
		t.Fatalf("expected error body to be preserved in result.Output, got: %q", result.Output)
	}
}

func TestWebFetchOmitsBinaryContent(t *testing.T) {
	t.Run("by content type", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "image/png")
			_, _ = writer.Write([]byte("\x89PNG\r\n\x1a\n"))
		}))
		t.Cleanup(server.Close)

		handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted, AllowLocalhost: true})
		result, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-bin-type", "web_fetch", map[string]any{
			"url": server.URL,
		}))
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if !strings.HasPrefix(result.Output, "[binary content omitted: image/png") {
			t.Fatalf("expected binary omitted message, got: %q", result.Output)
		}
	})

	t.Run("by null byte sniffing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = writer.Write([]byte("some text with a \x00 null byte"))
		}))
		t.Cleanup(server.Close)

		handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted, AllowLocalhost: true})
		result, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-bin-sniff", "web_fetch", map[string]any{
			"url": server.URL,
		}))
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if !strings.HasPrefix(result.Output, "[binary content omitted:") {
			t.Fatalf("expected binary omitted message, got: %q", result.Output)
		}
	})
}

func TestWebFetchBlocksPrivateAndMetadataSSRF(t *testing.T) {
	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted})

	blockedTargets := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:8080",
		"http://localhost:3000",
		"http://10.0.0.1/secret",
		"http://192.168.1.1/admin",
		"http://172.16.0.1/internal",
		"http://metadata.google.internal/computeMetadata/v1/",
	}

	for _, target := range blockedTargets {
		t.Run(target, func(t *testing.T) {
			_, err := handler.Execute(context.Background(), newJSONCall(t, "ssrf-test", "web_fetch", map[string]any{
				"url": target,
			}))
			if err == nil || !errors.Is(err, sandbox.ErrNetworkDenied) {
				t.Fatalf("expected ErrNetworkDenied for %q, got: %v", target, err)
			}
		})
	}

	// Cloud metadata must be blocked even if AllowLocalhost is true
	localHandler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted, AllowLocalhost: true})
	_, err := localHandler.Execute(context.Background(), newJSONCall(t, "meta-test", "web_fetch", map[string]any{
		"url": "http://169.254.169.254/latest/meta-data",
	}))
	if err == nil || !errors.Is(err, sandbox.ErrNetworkDenied) {
		t.Fatalf("expected ErrNetworkDenied for cloud metadata even with AllowLocalhost, got: %v", err)
	}
}
