// Package theme owns the single source of colour and semantic role styles for
// the relurpish terminal UI. Both tui and euclotui import theme; theme imports
// neither.
//
// A *Theme is threaded by value through construction and SetSize so it is
// testable and runtime-swappable. Agents may tint via an optional accent
// override that inherits all structure while replacing the accent colour.
package theme
