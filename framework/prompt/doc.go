// Package prompt implements the workspace prompt registry, parser, resolver,
// and provider interface for the relurpify framework.
//
// Prompts are file-resident, composable descriptions of model instructions.
// At runtime the registry resolves a prompt config against a RuntimeContext
// and returns a single assembled string for the LLM.
//
// Prompt files live in <workspace>/relurpify_cfg/prompts/ and are loaded at
// ayenitd.Open() time. Named agents register context providers during their
// own Initialize() calls.
//
// This package imports nothing from agents/ or named/.
package prompt
