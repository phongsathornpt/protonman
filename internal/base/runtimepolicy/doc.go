// Package runtimepolicy owns the canonical CLI runtime defaults.
//
// Retry timing mirrors the SDK-owned canonical defaults
// (protonsdk.DefaultRetryBaseBackoff/MaxBackoff/MaxAfter); change both sides
// together. Pinned by protonsdk_test.TestRetryZeroPolicyUsesCanonicalDefaults.
package runtimepolicy
