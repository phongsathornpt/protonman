//go:build !linux

package sandbox

// ProbeCapabilities reports no native Linux confinement features on non-Linux hosts.
func ProbeCapabilities() Capabilities {
	return Capabilities{}
}
