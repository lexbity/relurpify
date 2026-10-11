package ports

// ToolUnwrapper is the decoration contract for tool wrappers. Every decorator
// in the Build → Register chain MUST implement Unwrap so that registry-side
// stamping (sandbox scope, agent spec, permission manager) can reach the
// concrete tool regardless of how many wrappers layer on top. A wrapper that
// neither implements Unwrap nor forwards the stamp interfaces silently breaks
// enforcement — the composition-root stamp contract test exists to catch that.
type ToolUnwrapper interface {
	Unwrap() Tool
}

// MaxUnwrapDepth bounds unwrapTool recursion. A decorator chain deeper than
// this is a pathology; the depth cap treats the tool as atomic and the
// composition gate is what notices the missed stamp.
const MaxUnwrapDepth = 32
