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

// Trigger association names are the only recipe-local association keys allowed
// on a trigger declaration.
const (
	TriggerAssociationFamily  = "family"
	TriggerAssociationKeyword = "keyword"
	TriggerAssociationHandoff = "handoff"
)

// SupportedTriggerAssociations lists the deterministic trigger-local association
// keys that may appear under a trigger declaration.
var SupportedTriggerAssociations = []string{
	TriggerAssociationFamily,
	TriggerAssociationKeyword,
	TriggerAssociationHandoff,
}

// IsSupportedTriggerAssociation reports whether the provided association name
// is allowed in the trigger surface.
func IsSupportedTriggerAssociation(name string) bool {
	for _, accepted := range SupportedTriggerAssociations {
		if name == accepted {
			return true
		}
	}
	return false
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
