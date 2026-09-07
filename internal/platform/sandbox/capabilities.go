package sandbox

// Capabilities reports host features relevant to process confinement.
// A capability being reported does not bypass host LSM or container policy;
// execution may still fail closed when a security policy denies the operation.
type Capabilities struct {
	Native         bool
	LandlockABI    int
	UserNamespaces bool
	Bubblewrap     bool
}

// SupportsLandlock reports whether the kernel exposes the Landlock ABI.
func (c Capabilities) SupportsLandlock() bool {
	return c.LandlockABI > 0
}
