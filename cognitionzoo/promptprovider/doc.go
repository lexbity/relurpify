// Package promptprovider implements context providers for generic execution
// paradigms (react, pipeline). Providers are registered with the workspace
// prompt registry during named-agent Initialize() calls via RegisterAll.
//
// Each provider maps to one from:provider block in a .prompt file.
// Provider names use the paradigm prefix: "react.*", "pipeline.*".
package promptprovider
