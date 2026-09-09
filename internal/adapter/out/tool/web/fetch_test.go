package webtool

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/platform/sandbox"
)

type staticResolver struct {
	ips []net.IP
	err error
}

func (r staticResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return r.ips, r.err
}

type recordingDialer struct {
	addresses []string
	err       error
}

func (d *recordingDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.addresses = append(d.addresses, address)
	return nil, d.err
}

func newJSONCall(t *testing.T, id, name string, input map[string]any) tool.Call {
	t.Helper()
	arguments, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	call, err := tool.NewCall(id, name, arguments)
	if err != nil {
		t.Fatal(err)
	}
	return call
}

func TestWebFetchHonorsBlockedNetworkPolicy(t *testing.T) {
	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkBlocked})
	_, err := handler.Execute(context.Background(), newJSONCall(t, "fetch-1", "web_fetch", map[string]any{
		"url": "https://example.com",
	}))
	if !errors.Is(err, sandbox.ErrNetworkDenied) {
		t.Fatalf("Execute() error = %v, want network denied", err)
	}
}

func TestWebFetchTransportRejectsPrivateResolvedDestination(t *testing.T) {
	dialer := &recordingDialer{err: errors.New("dial should not run")}
	transport := newWebFetchTransport(
		sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted},
		staticResolver{ips: []net.IP{net.ParseIP("10.0.0.8")}},
		dialer,
	)
	_, err := transport.DialContext(context.Background(), "tcp", "example.com:443")
	if !errors.Is(err, sandbox.ErrNetworkDenied) {
		t.Fatalf("DialContext() error = %v, want network denied", err)
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("unsafe destination reached dialer: %v", dialer.addresses)
	}
}

func TestWebFetchTransportPinsValidatedResolvedIP(t *testing.T) {
	sentinel := errors.New("stop after recording dial")
	dialer := &recordingDialer{err: sentinel}
	transport := newWebFetchTransport(
		sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted},
		staticResolver{ips: []net.IP{net.ParseIP("93.184.216.34")}},
		dialer,
	)
	if transport.Proxy != nil {
		t.Fatal("web_fetch transport must ignore environment HTTP proxies")
	}
	_, err := transport.DialContext(context.Background(), "tcp", "example.com:443")
	if !errors.Is(err, sentinel) {
		t.Fatalf("DialContext() error = %v, want sentinel", err)
	}
	if len(dialer.addresses) != 1 || dialer.addresses[0] != "93.184.216.34:443" {
		t.Fatalf("dialed addresses = %v, want pinned resolved IP", dialer.addresses)
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
	if handler.Definition().Kind != tool.KindWeb {
		t.Fatalf("kind = %s, want %s", handler.Definition().Kind, tool.KindWeb)
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

func TestWebFetchRedirectRejectsPrivateTarget(t *testing.T) {
	handler := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted}).(webFetchHandler)
	request, err := http.NewRequest(http.MethodGet, "http://10.0.0.1/internal", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.client.CheckRedirect(request, []*http.Request{{}}); !errors.Is(err, sandbox.ErrNetworkDenied) {
		t.Fatalf("CheckRedirect() error = %v, want network denied", err)
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
	if receivedUA != buildinfo.WebUserAgent() {
		t.Errorf("expected User-Agent %q, got %q", buildinfo.WebUserAgent(), receivedUA)
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
	if err == nil || !strings.Contains(err.Error(), "web status 404: Not Found") {
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

func TestWebSearchUsesCanonicalCapability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		if got := req.URL.Query().Get("q"); got != "golang concurrency" {
			t.Fatalf("query = %q", got)
		}
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte(`<html><body>
<a class="result__a" href="https://example.com/one">First &amp; Result</a>
<a class="result__a" href="/l/?uddg=https%3A%2F%2Fexample.org%2Ftwo">Second Result</a>
</body></html>`))
	}))
	t.Cleanup(server.Close)

	handler := NewWebFetch(
		sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted, AllowLocalhost: true},
		WithWebSearchEndpoint(server.URL),
	)
	result, err := handler.Execute(context.Background(), newJSONCall(t, "search-1", "web", map[string]any{
		"action": "search",
		"query":  "golang concurrency",
		"limit":  2,
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"First & Result", "https://example.com/one", "Second Result", "https://example.org/two"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output missing %q: %s", want, result.Output)
		}
	}
	if result.ToolName != "web" {
		t.Fatalf("ToolName = %q, want web", result.ToolName)
	}
}

func TestWebDefinitionPublishesFetchAndSearchActions(t *testing.T) {
	def := NewWebFetch(sandbox.NetworkPolicy{Mode: sandbox.NetworkUnrestricted}).Definition()
	if def.Name != "web" {
		t.Fatalf("name = %q", def.Name)
	}
	encoded, err := json.Marshal(def.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, want := range []string{`"fetch"`, `"search"`, `"query"`, `"url"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("schema missing %s: %s", want, text)
		}
	}
}
