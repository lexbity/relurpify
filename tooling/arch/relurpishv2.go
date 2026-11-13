package arch

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// CheckRelurpishV2Source checks the production interaction boundary. It uses
// syntax and resolved import names, so renaming an os import cannot hide writes.
func CheckRelurpishV2Source(filename string, source []byte) ([]string, error) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, filename, source, 0)
	if err != nil {
		return nil, err
	}
	imports := make(map[string]string)
	for _, item := range file.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			return nil, err
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if item.Name != nil {
			name = item.Name.Name
		}
		if name == "." {
			return []string{fmt.Sprintf("%s: unqualified import", set.Position(item.Pos()))}, nil
		}
		imports[name] = path
	}
	var violations []string
	report := func(position token.Pos, reason string) {
		violations = append(violations, fmt.Sprintf("%s: %s", set.Position(position), reason))
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.TypeSpec:
			if value.Assign.IsValid() {
				report(value.Pos(), "type alias")
			}
		case *ast.FuncDecl:
			if strings.Contains(value.Name.Name, "ForTest") || value.Name.Name == "NewTestRootModel" {
				report(value.Pos(), "production test mutation API")
			}
		case *ast.SelectorExpr:
			if value.Sel.Name == "FieldByName" {
				report(value.Pos(), "reflective widget field access")
			}
			if name, ok := value.X.(*ast.Ident); ok && (imports[name.Name] == "os" || imports[name.Name] == "syscall") {
				switch value.Sel.Name {
				case "Getenv", "LookupEnv", "Environ", "Setenv", "Unsetenv", "Clearenv", "ExpandEnv":
					report(value.Pos(), "environment access outside userconfig")
				}
			}
			if strings.Contains(filename, "euclotui/") {
				if name, ok := value.X.(*ast.Ident); ok && imports[name.Name] == "os" {
					switch value.Sel.Name {
					case "WriteFile", "Create", "OpenFile", "Remove", "RemoveAll", "Rename":
						report(value.Pos(), "filesystem mutation in execution view")
					}
				}
			}
		case *ast.BasicLit:
			if value.Kind == token.STRING {
				text, err := strconv.Unquote(value.Value)
				if err == nil && strings.Contains(text, "not yet implemented as palette command") {
					report(value.Pos(), "unimplemented advertised command")
				}
			}
		}
		return true
	})
	return violations, nil
}

// MissingRelurpishV2Tests rejects a successful command that lists no tests.
func MissingRelurpishV2Tests(listing string) []string {
	found := make(map[string]bool)
	for _, name := range strings.Split(listing, "\n") {
		found[strings.TrimSpace(name)] = true
	}
	var missing []string
	for _, name := range []string{
		"TestV2PlainTextEverySurface", "TestV2DraftSurvivesNavigation",
		"TestV2ComposerAcknowledgementRejectsRecreatedDraft", "TestV2ComposerHistoryAndPalettePreserveDraft",
		"TestV2QuitRequiresFocusedReviewAndPreservesDraft", "TestV2HelpOwnsPrintableAndPasteKeys", "TestV2BackRestoresCanvasReadingAndFocus",
		"TestV2CanonicalRecipeCommandsUseOneAsyncWorkshop", "TestV2RecipeEscapeRetainsInvalidSourceDraft", "TestV2LateRecipeRunCannotReplaceChosenCanvas",
		"TestV2CircuitDiagramKeepsGeometryAndHonestEdges", "TestV2FileCompletionIsExplicitBoundedAndSpanChecked", "TestV2FileCompletionTokenBytesAndCancellation",
		"TestV2AgentChooserConsumesKeysBeforeSettings", "TestV2AttentionPasteCannotEditUnderlyingRequest", "TestV2RecipeNewSaveReopenAndConflict",
		"TestV2KeybindingRejectsUnrepresentableChords", "TestV2HelpSupportsScopedDrilldown",
		"TestV2ApprovalCannotStealTyping", "TestV2PasteDoesNotSubmit",
		"TestV2ViewIsPure", "TestV2DoctorWithoutRuntime",
		"TestV2TextBatchesAreNeverControlKeys",
		"TestV2CommandArguments", "TestV2LiteralRequestRetainsBlockedDraft",
		"TestV2CircuitSelectionSurvivesProgress", "TestV2CircuitHostRetainsDraftAndFocus",
		"TestV2CircuitBoundsAndNoInventedStatus",
		"TestV2CommandMigrationInventory", "TestV2HelpUsesCurrentCatalogWithoutRuntime",
		"TestV2HostCleanupIsSharedAndIdempotent", "TestV2HostCleanupContinuesAfterResourcePanic",
		"TestV2KeybindingConflictReviewSaveAndIsolation", "TestV2KeybindingSaveFailureRetainsActiveMap", "TestV2KeybindingResetRequiresReviewAndCancelPreservesMap",
		"TestV2WorkshopSourceEditsPreserveUntouchedBytes", "TestV2WorkshopConnectionsExcludeFutureCaptures",
		"TestV2WorkshopCompleteAuthorConnectBindSaveTrigger", "TestV2WorkshopInvalidSourceCannotSaveAndUndoRecovers", "TestV2WorkshopSaveConflictDoesNotOverwrite",
		"TestV2DemoDecisionUsesSharedHostAndPreservesDraft", "TestV2DemoInitializationReviewCancelAndApply", "TestV2DemoClockCancellationAndSnapshotIsolation",
		"TestV2CircuitConsumesActualEucloEmitter", "TestV2CircuitInvalidTelemetryRemainsVisible",
		"TestV2ContextCommandsChangeFutureSelectionWithoutFakeRuntimeSuccess", "TestV2ActivityDistinguishesMissingEstimatedAndInvalidUsage", "TestV2ActivityUsesTelemetryWithoutRetainingPayloads", "TestV2ActivityRetainsReadingAndBounds", "TestV2ActivityHostFilterAndDraftSurviveProgress",
		"TestV2ObservationsRejectPreviousWorkspaceAndSubscription", "TestV2AttentionRejectsStaleWorkspaceAndResolutionFailure",
		"TestV2HostReplacesSubscriptionsWithoutAccumulatingOwners",
		"TestV2ProviderReviewSaveActivateAndRetainedDraft", "TestV2ProviderActivationFailureRetainsLastGoodState", "TestV2ProviderStaleProbeAndSaveFailureRetainDraft", "TestV2ProviderRejectsQuestionableFields", "TestV2LegacyProviderNavigationNeverProbes", "TestV2DemoActivityIsLabeledAndDoesNotDuplicate",
	} {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

func MissingRelurpishV2IntegrationTests(listing string) []string {
	var missing []string
	for _, name := range []string{"TestV2DoctorWithoutRuntime", "TestV2PTYFocusAndBounds", "TestV2PTYDemoCommandDraftRoundTrip", "TestV2PTYActivityProviderAndDraft", "TestV2PTYRecipeAuthorSaveRunAndQuit"} {
		found := false
		for _, line := range strings.Split(listing, "\n") {
			if strings.TrimSpace(line) == name {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, name)
		}
	}
	return missing
}
