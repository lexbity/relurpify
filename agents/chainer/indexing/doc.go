// Package indexing provides code snippet indexing and retrieval for ChainerAgent.
//
// # Overview
//
// Phase 7: Semantic indexing of code evaluated during link execution, enabling
// retrieval of previously-analyzed code to contextualize future tool invocations.
//
// # Components
//
// CodeIndex stores evaluated code snippets with rich metadata (file path, language,
// symbols, dependencies). Retrieval queries code by similarity, symbol presence,
// or metadata filters. Integration wires the CodeIndex into ChainerAgent to capture
// evaluation results and provide context for subsequent links.
//
// # Evaluation Snapshots
//
// Each code evaluation creates an IndexedCodeSnippet with:
//   - Original source code
//   - Metadata (file path, language, symbols found)
//   - Link name that evaluated it
//   - Timestamp and task ID
//   - Optional analysis result (parsing, evaluation, diagnosis)
//
// # Retrieval Strategies
//
// RetrievalQuery supports multiple strategies:
//   - Symbol-based: Find snippets containing function/type names
//   - Path-based: Find snippets from specific files or patterns
//   - Content-based: Find snippets by keyword or pattern
//   - Link-based: Find snippets evaluated by specific link
//   - Time-based: Find snippets from recent evaluations
package indexing
