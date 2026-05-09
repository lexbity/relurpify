package thoughtrecipe

// ThoughtRecipeSourceRoot is the only supported workspace root for Euclo thoughtrecipe files.
const ThoughtRecipeSourceRoot = "relurpify_cfg/euclo"

// ThoughtRecipeIdentityHeader is the only supported identity header for Euclo thoughtrecipes.
const ThoughtRecipeIdentityHeader = "thoughtrecipe"

// AcceptedThoughtRecipeExtensions are the only accepted thoughtrecipe source file extensions.
var AcceptedThoughtRecipeExtensions = []string{".erpe", ".euclo", ".thoughtrecipe"}

// AllowedTopLevelDeclarations freezes the current top-level declaration surface
// for the Euclo thoughtrecipe DSL.
var AllowedTopLevelDeclarations = []string{
	"thoughtrecipe",
	"trigger",
	"input",
	"type",
	"agent",
	"run",
	"route",
	"delegate",
	"ask",
	"pipeline",
}

// SupportedNamespaces freezes the state namespaces that the DSL may target.
var SupportedNamespaces = []string{
	"input.*",
	"state.*",
	"scratch.*",
	"user.*",
	"output.*",
}

// IsAcceptedThoughtRecipeExtension reports whether the extension is supported.
func IsAcceptedThoughtRecipeExtension(ext string) bool {
	for _, accepted := range AcceptedThoughtRecipeExtensions {
		if ext == accepted {
			return true
		}
	}
	return false
}
