package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/platform/sandbox"
)

const (
	Repository = "phongsathornpt/protonman"
	githubAPI  = "https://api.github.com"
	githubWeb  = "https://github.com"
)

var updateOrigins = []sandbox.Origin{
	mustOrigin("https://api.github.com"),
	mustOrigin("https://github.com"),
	mustOrigin("https://objects.githubusercontent.com"),
	mustOrigin("https://release-assets.githubusercontent.com"),
}

func mustOrigin(raw string) sandbox.Origin {
	origin, err := sandbox.ParseOrigin(raw)
	if err != nil {
		panic(fmt.Sprintf("update origin %q: %v", raw, err))
	}
	return origin
}

func updatePolicy() sandbox.NetworkPolicy {
	return sandbox.NetworkPolicy{Mode: sandbox.NetworkAllowlist, Allowed: updateOrigins}
}

func newUpdateClient() *http.Client {
	policy := updatePolicy()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("split update dial address %q: %w", address, err)
		}
		var ips []net.IP
		if literal := net.ParseIP(host); literal != nil {
			ips = []net.IP{literal}
		} else {
			resolved, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve update host %q: %w", host, err)
			}
			ips = resolved
		}
		if err := policy.ValidateResolvedAddresses(ips); err != nil {
			return nil, err
		}
		dialer := &net.Dialer{}
		var lastErr error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, fmt.Errorf("dial update destination %q: %w", address, lastErr)
	}
	return &http.Client{
		Timeout:   runtimepolicy.UpdateHTTPTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to unsupported scheme %q", req.URL.Scheme)
			}
			return policy.AllowURL(req.URL.String())
		},
	}
}

func authToken() string {
	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func latestTagFromAPI(ctx context.Context, client *http.Client) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPI+"/repos/"+Repository+"/releases/latest", nil)
	if err != nil {
		return "", fmt.Errorf("build latest release request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", buildinfo.WebUserAgent())
	if token := authToken(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("resolve latest release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolve latest release: unexpected status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, runtimepolicy.UpdateMaxChecksumsBytes))
	if err != nil {
		return "", fmt.Errorf("read latest release: %w", err)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode latest release: %w", err)
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return "", fmt.Errorf("latest release did not contain a version tag")
	}
	return ValidateTag(payload.TagName)
}

func latestTagFromRedirect(ctx context.Context, client *http.Client) (string, error) {
	noRedirect := *client
	noRedirect.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, githubWeb+"/"+Repository+"/releases/latest", nil)
	if err != nil {
		return "", fmt.Errorf("build latest redirect request: %w", err)
	}
	request.Header.Set("User-Agent", buildinfo.WebUserAgent())
	response, err := noRedirect.Do(request)
	if err != nil {
		return "", fmt.Errorf("resolve latest release redirect: %w", err)
	}
	defer response.Body.Close()
	location := strings.TrimSpace(response.Header.Get("Location"))
	if location == "" {
		return "", fmt.Errorf("latest release redirect did not contain a location")
	}
	if !strings.HasPrefix(location, githubWeb+"/"+Repository+"/releases/tag/") {
		return "", fmt.Errorf("unexpected latest release redirect: %q", location)
	}
	tag := strings.TrimSpace(location[strings.LastIndex(location, "/")+1:])
	return ValidateTag(tag)
}

func ResolveLatestTag(ctx context.Context, client *http.Client) (string, error) {
	if client == nil {
		client = newUpdateClient()
	}
	ctx, cancel := context.WithTimeout(ctx, runtimepolicy.UpdateHTTPTimeout)
	defer cancel()
	if authToken() != "" {
		if tag, err := latestTagFromAPI(ctx, client); err == nil {
			return tag, nil
		}
	}
	if tag, err := latestTagFromRedirect(ctx, client); err == nil {
		return tag, nil
	} else if authToken() == "" {
		return "", err
	}
	return latestTagFromAPI(ctx, client)
}

func tagExists(ctx context.Context, client *http.Client, tag string) error {
	ctx, cancel := context.WithTimeout(ctx, runtimepolicy.UpdateHTTPTimeout)
	defer cancel()
	url := githubWeb + "/" + Repository + "/releases/download/" + tag + "/checksums.txt"
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("build release probe: %w", err)
	}
	request.Header.Set("User-Agent", buildinfo.WebUserAgent())
	if token := authToken(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("probe release %s: %w", tag, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("release %s not found (status %d)", tag, response.StatusCode)
}
