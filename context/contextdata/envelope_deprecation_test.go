//go:build ignore

package contextdata

// This file documents the intended end state after phase 6:
// - SetWorkingValue and GetWorkingValue are deprecated.
// - New code should use SetTyped, GetTyped, or TypedOverlay.
// - Handle-scoped access should be represented with explicit typed keys.
//
// The file is excluded from normal builds so it serves as a stable reference
// without affecting the test suite.
