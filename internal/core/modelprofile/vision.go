package modelprofile

import "fmt"

// VisionTokenScheme identifies how a model accounts image input in context.
// Preparation limits and token accounting live in one policy so transport
// encoding cannot accidentally become the model-context contract.
type VisionTokenScheme string

const (
	VisionTokenPatch32         VisionTokenScheme = "patch32"
	VisionTokenAnthropicPixels VisionTokenScheme = "anthropic_pixels"
	VisionTokenGeminiTiles     VisionTokenScheme = "gemini_tiles"
)

// VisionPolicy describes the model-facing image budget. MaxPatches is also
// used as an area bound during preparation even for token schemes that do not
// themselves charge one token per patch.
type VisionPolicy struct {
	MaxDimension   int
	MaxPatches     int
	PatchSize      int
	MaxOutputBytes int
	TokenScheme    VisionTokenScheme
	FallbackTokens int
}

const (
	defaultVisionMaxDimension   = 2048
	defaultVisionMaxPatches     = 2500
	defaultVisionPatchSize      = 32
	defaultVisionMaxOutputBytes = 10 * 1024 * 1024
	defaultVisionFallbackTokens = 2500
)

// DefaultVisionPolicy is the conservative OpenAI-compatible fallback used when
// a resolved model profile does not publish more specific image semantics.
func DefaultVisionPolicy() VisionPolicy {
	return VisionPolicy{
		MaxDimension:   defaultVisionMaxDimension,
		MaxPatches:     defaultVisionMaxPatches,
		PatchSize:      defaultVisionPatchSize,
		MaxOutputBytes: defaultVisionMaxOutputBytes,
		TokenScheme:    VisionTokenPatch32,
		FallbackTokens: defaultVisionFallbackTokens,
	}
}

// EffectiveVisionPolicy fills omitted fields from the conservative default.
func EffectiveVisionPolicy(resolved Resolved) VisionPolicy {
	policy := resolved.VisionPolicy
	defaults := DefaultVisionPolicy()
	if policy.MaxDimension <= 0 {
		policy.MaxDimension = defaults.MaxDimension
	}
	if policy.MaxPatches <= 0 {
		policy.MaxPatches = defaults.MaxPatches
	}
	if policy.PatchSize <= 0 {
		policy.PatchSize = defaults.PatchSize
	}
	if policy.MaxOutputBytes <= 0 {
		policy.MaxOutputBytes = defaults.MaxOutputBytes
	}
	if policy.TokenScheme == "" {
		policy.TokenScheme = defaults.TokenScheme
	}
	if policy.FallbackTokens <= 0 {
		policy.FallbackTokens = defaults.FallbackTokens
	}
	return policy
}

func validateVisionPolicy(policy VisionPolicy) error {
	if policy == (VisionPolicy{}) {
		return nil
	}
	if policy.MaxDimension < 0 || policy.MaxPatches < 0 || policy.PatchSize < 0 || policy.MaxOutputBytes < 0 || policy.FallbackTokens < 0 {
		return fmt.Errorf("vision policy numeric limits must be non-negative")
	}
	switch policy.TokenScheme {
	case "", VisionTokenPatch32, VisionTokenAnthropicPixels, VisionTokenGeminiTiles:
	default:
		return fmt.Errorf("invalid vision token scheme %q", policy.TokenScheme)
	}
	return nil
}
