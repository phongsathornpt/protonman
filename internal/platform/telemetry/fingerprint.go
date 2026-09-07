package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
)

// Fingerprint returns a short stable identifier for correlating debug events
// without writing the original value to logs.
func Fingerprint(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}
