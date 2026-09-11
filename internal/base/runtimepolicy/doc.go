// Package runtimepolicy owns the canonical CLI runtime defaults.
//
// The model retry values are the application policy passed explicitly to the
// provider-neutral SDK at composition time. The SDK retains independent
// library fallbacks for callers that do not provide a policy.
package runtimepolicy
