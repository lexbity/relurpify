// Package validation provides schema-based output validation for ChainerAgent stages.
//
// # Schema Types
//
// Validator supports two schema modes:
//   - JSONSchema: JSON Schema (https://json-schema.org/) validation
//   - CustomValidator: User-provided validation function
//
// # Validation Flow
//
// LinkStage.Validate() uses Validator to check LLM output:
//  1. Check if schema defined (skip if not)
//  2. Attempt parse (JSON decode for JSONSchema mode)
//  3. Run schema validation rules
//  4. Return ValidationError on failure (triggers retry)
//
// # Usage Example
//
// Define schema on Link:
//
//	Link{
//	    Name: "extract",
//	    Schema: `{"type": "object", "properties": {"name": {"type": "string"}}}`,
//	}
//
// LinkStage automatically validates output against schema.
package validation
