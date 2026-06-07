// Package templates resolves agent and skill prompt templates from the
// filesystem, providing a caching loader for text/template files used to
// build LLM prompts at runtime.
//
// TemplateResolver searches the configured template directories in priority
// order (workspace-local overrides first, then shared defaults) and caches
// parsed templates to avoid repeated disk reads across agent iterations.
package templates
