// Package sandbox defines OS confinement profiles and child-network policy.
package sandbox

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
)

// ErrInvalidProfile indicates that a sandbox profile cannot be used.
var ErrInvalidProfile = errors.New("invalid sandbox profile")

// ErrNetworkDenied indicates that a URL is blocked by the child-network policy.
var ErrNetworkDenied = errors.New("network denied")

// Name identifies a built-in sandbox profile.
type Name uint8

const (
	// NameUnknown is the invalid zero value.
	NameUnknown Name = iota
	// NameOff disables OS confinement.
	NameOff
	// NameWorkspace confines writes to the workspace and leaves network open.
	NameWorkspace
	// NameReadOnly denies workspace writes and blocks child network.
	NameReadOnly
	// NameStrict confines writes to the workspace and blocks child network.
	NameStrict
)

// String returns the configuration spelling of a profile name.
func (n Name) String() string {
	switch n {
	case NameOff:
		return "off"
	case NameWorkspace:
		return "workspace"
	case NameReadOnly:
		return "read-only"
	case NameStrict:
		return "strict"
	default:
		return "unknown"
	}
}

// Valid reports whether the profile name is a recognized non-zero profile.
func (n Name) Valid() bool {
	switch n {
	case NameOff, NameWorkspace, NameReadOnly, NameStrict:
		return true
	default:
		return false
	}
}

// MarshalText implements encoding.TextMarshaler.
func (n Name) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (n *Name) UnmarshalText(text []byte) error {
	parsed, err := ParseName(string(text))
	if err != nil {
		return err
	}
	*n = parsed
	return nil
}

// ParseName parses built-in profile names. Unknown names and empty strings fail closed.
func ParseName(value string) (Name, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "none":
		return NameOff, nil
	case "workspace":
		return NameWorkspace, nil
	case "read-only", "readonly", "read_only":
		return NameReadOnly, nil
	case "strict":
		return NameStrict, nil
	default:
		return NameUnknown, fmt.Errorf("%w: %q", ErrInvalidProfile, value)
	}
}

// NetworkMode is the child-process egress policy.
type NetworkMode uint8

const (
	// NetworkUnknown is the invalid zero value.
	NetworkUnknown NetworkMode = iota
	// NetworkUnrestricted allows any destination.
	NetworkUnrestricted
	// NetworkBlocked denies every destination.
	NetworkBlocked
	// NetworkAllowlist allows only exact http(s) origins.
	NetworkAllowlist
)

// String returns the configuration spelling of a network mode.
func (m NetworkMode) String() string {
	switch m {
	case NetworkUnrestricted:
		return "unrestricted"
	case NetworkBlocked:
		return "blocked"
	case NetworkAllowlist:
		return "allowlist"
	default:
		return "unknown"
	}
}

// Valid reports whether the network mode is a recognized non-zero mode.
func (m NetworkMode) Valid() bool {
	return m == NetworkUnrestricted || m == NetworkBlocked || m == NetworkAllowlist
}

// ParseNetworkMode parses network egress modes.
func ParseNetworkMode(value string) (NetworkMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "unrestricted", "open":
		return NetworkUnrestricted, nil
	case "blocked", "deny", "none":
		return NetworkBlocked, nil
	case "allowlist", "whitelist":
		return NetworkAllowlist, nil
	default:
		return NetworkUnknown, fmt.Errorf("%w: invalid network mode %q", ErrInvalidProfile, value)
	}
}

// MarshalText implements encoding.TextMarshaler.
func (m NetworkMode) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *NetworkMode) UnmarshalText(text []byte) error {
	parsed, err := ParseNetworkMode(string(text))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// Origin is an exact http(s) origin: scheme, hostname, and port.
type Origin struct {
	Scheme string
	Host   string
	Port   string
}

// String returns scheme://host:port.
func (o Origin) String() string {
	return o.Scheme + "://" + o.Host + ":" + o.Port
}

// ParseOrigin accepts only http(s) origins with a DNS hostname.
func ParseOrigin(raw string) (Origin, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Origin{}, fmt.Errorf("%w: origin is empty", ErrInvalidProfile)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return Origin{}, fmt.Errorf("%w: parse origin: %v", ErrInvalidProfile, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Origin{}, fmt.Errorf("%w: origin scheme must be http or https", ErrInvalidProfile)
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		return Origin{}, fmt.Errorf("%w: origin host is required", ErrInvalidProfile)
	}
	if net.ParseIP(host) != nil {
		return Origin{}, fmt.Errorf("%w: ip literals are not allowed", ErrInvalidProfile)
	}
	if strings.ContainsAny(host, " \t\r\n/") {
		return Origin{}, fmt.Errorf("%w: origin host is invalid", ErrInvalidProfile)
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	path := parsed.EscapedPath()
	if path != "" && path != "/" {
		return Origin{}, fmt.Errorf("%w: origin must not include a path", ErrInvalidProfile)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return Origin{}, fmt.Errorf("%w: origin must not include userinfo, query, or fragment", ErrInvalidProfile)
	}
	return Origin{Scheme: parsed.Scheme, Host: host, Port: port}, nil
}

// NetworkPolicy evaluates child website egress.
type NetworkPolicy struct {
	Mode           NetworkMode
	Allowed        []Origin
	AllowLocalhost bool
}

// AllowURL reports whether a URL may be fetched under this policy.
func (p NetworkPolicy) AllowURL(raw string) error {
	switch p.Mode {
	case NetworkBlocked:
		return fmt.Errorf("%w: child network is blocked", ErrNetworkDenied)
	case NetworkAllowlist:
		origin, err := ParseOrigin(originOf(raw))
		if err != nil {
			return fmt.Errorf("%w: %v", ErrNetworkDenied, err)
		}
		if slices.Contains(p.Allowed, origin) {
			return nil
		}
		return fmt.Errorf("%w: origin %s is not allowlisted", ErrNetworkDenied, origin)
	case NetworkUnrestricted:
		if len(p.Allowed) > 0 {
			if origin, err := ParseOrigin(originOf(raw)); err == nil && slices.Contains(p.Allowed, origin) {
				return nil
			}
		}
		return isBlockedTarget(raw, p.AllowLocalhost)
	default:
		return fmt.Errorf("%w: invalid network mode", ErrNetworkDenied)
	}
}

func isBlockedTarget(raw string, allowLocalhost bool) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return fmt.Errorf("%w: missing host in URL", ErrNetworkDenied)
	}

	if hostname == "metadata.google.internal" || hostname == "instance-data" {
		return fmt.Errorf("%w: cloud metadata endpoint %s is blocked", ErrNetworkDenied, hostname)
	}

	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") || strings.HasSuffix(hostname, ".local") {
		if !allowLocalhost {
			return fmt.Errorf("%w: local address %s is blocked", ErrNetworkDenied, hostname)
		}
		return nil
	}

	if ip := net.ParseIP(hostname); ip != nil {
		return validateDestinationIP(ip, allowLocalhost)
	}
	return nil
}

// ValidateResolvedIPs applies the network policy to concrete addresses returned
// by DNS. Every answer must be safe; mixed public/private results fail closed.
func (p NetworkPolicy) ValidateResolvedIPs(raw string, ips []net.IP) error {
	if err := p.AllowURL(raw); err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("%w: host resolved to no addresses", ErrNetworkDenied)
	}
	for _, ip := range ips {
		if err := validateDestinationIP(ip, p.AllowLocalhost); err != nil {
			return err
		}
	}
	return nil
}

func validateDestinationIP(ip net.IP, allowLocalhost bool) error {
	if ip == nil {
		return fmt.Errorf("%w: invalid destination address", ErrNetworkDenied)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("%w: link-local/metadata address %s is blocked", ErrNetworkDenied, ip)
	}
	if ip.IsLoopback() {
		if allowLocalhost {
			return nil
		}
		return fmt.Errorf("%w: loopback address %s is blocked", ErrNetworkDenied, ip)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("%w: unspecified address %s is blocked", ErrNetworkDenied, ip)
	}
	if ip.IsPrivate() {
		return fmt.Errorf("%w: private network address %s is blocked", ErrNetworkDenied, ip)
	}
	return nil
}

func originOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimSpace(raw)
	}
	return parsed.Scheme + "://" + parsed.Host
}

// Profile is a resolved confinement plan for one workspace.
type Profile struct {
	Name            Name
	Workspace       string
	RestrictNetwork bool
	ReadOnly        bool
	Network         NetworkPolicy
	AllowWriteRoot  bool
}

// NewProfile builds a built-in profile for a workspace root.
func NewProfile(name Name, workspace string) (Profile, error) {
	if name == NameUnknown {
		return Profile{}, fmt.Errorf("%w: unknown profile", ErrInvalidProfile)
	}
	workspace = strings.TrimSpace(workspace)
	if name != NameOff && workspace == "" {
		return Profile{}, fmt.Errorf("%w: workspace is required", ErrInvalidProfile)
	}
	profile := Profile{
		Name:      name,
		Workspace: workspace,
		Network:   NetworkPolicy{Mode: NetworkUnrestricted, Allowed: []Origin{}},
	}
	switch name {
	case NameReadOnly:
		profile.RestrictNetwork = true
		profile.ReadOnly = true
		profile.Network.Mode = NetworkBlocked
	case NameStrict:
		profile.RestrictNetwork = true
		profile.AllowWriteRoot = true
		profile.Network.Mode = NetworkBlocked
	case NameWorkspace:
		profile.AllowWriteRoot = true
	}
	return profile, nil
}

// Confines reports whether the profile requests OS confinement.
func (p Profile) Confines() bool {
	return p.Name != NameOff && p.Name != NameUnknown
}
