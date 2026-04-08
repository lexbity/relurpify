package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestArchaeoTab_Registration
// ---------------------------------------------------------------------------

// TestArchaeoTab_Registration verifies that the archaeo tab is registered in
// the tab registry and is visible for the euclo agent.
func TestArchaeoTab_Registration(t *testing.T) {
	reg := NewTabRegistry()
	registerEucloTabs(reg)

	tabs := reg.TabsForAgent("euclo")
	var found bool
	var archaeoDef TabDefinition
	for _, tab := range tabs {
		if tab.ID == TabArchaeo {
			found = true
			archaeoDef = tab
			break
		}
	}
	require.True(t, found, "archaeo tab should be visible for euclo agent")
	require.Equal(t, "archaeo", archaeoDef.Label)

	// Verify subtabs are registered.
	ids := make([]SubTabID, len(archaeoDef.SubTabs))
	for i, st := range archaeoDef.SubTabs {
		ids[i] = st.ID
	}
	require.Contains(t, ids, SubTabArchaeoPlan, "plan subtab should be registered")
	require.Contains(t, ids, SubTabArchaeoExplore, "explore subtab should be registered")
}

// TestArchaeoTab_NotVisibleForOtherAgents verifies the archaeo tab is hidden
// from non-euclo agents.
func TestArchaeoTab_NotVisibleForOtherAgents(t *testing.T) {
	reg := NewTabRegistry()
	registerEucloTabs(reg)

	tabs := reg.TabsForAgent("other-agent")
	for _, tab := range tabs {
		require.NotEqual(t, TabArchaeo, tab.ID, "archaeo tab should not be visible for other agents")
	}
}

// ---------------------------------------------------------------------------
// TestExploreSubtab_RendersFullWidth
// ---------------------------------------------------------------------------

// TestExploreSubtab_RendersFullWidth verifies that the explore subtab renders
// without a sidebar split — there should be no sidebar decoration column.
func TestExploreSubtab_RendersFullWidth(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoExplore)

	view := p.View()
	// A sidebar split would produce "│" or "┌" column separators; there should be none.
	require.NotContains(t, view, "│", "explore view should not have sidebar column separator")
	require.Contains(t, view, "explore", "explore header should be present")
}

// ---------------------------------------------------------------------------
// TestExploreSubtab_FrameRendering
// ---------------------------------------------------------------------------

// TestExploreSubtab_FrameRendering verifies that blob proposal entries injected
// via ArchaeoExploreMsg are rendered with [stage] action badges.
func TestExploreSubtab_FrameRendering(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoExplore)

	entries := []ExploreEntry{
		{
			Kind: ExploreEntryBlob,
			Blob: BlobEntry{ID: "t1", Kind: BlobTension, Title: "naming-conv", Description: "Inconsistent naming"},
		},
		{
			Kind: ExploreEntryBlob,
			Blob: BlobEntry{ID: "p1", Kind: BlobPattern, Title: "error-handling"},
		},
	}
	updated, _ := p.Update(ArchaeoExploreMsg{Entries: entries})
	p = updated.(*ArchaeoPane)

	view := p.View()
	require.Contains(t, view, "naming-conv", "tension title should be rendered")
	require.Contains(t, view, "[stage]", "unstaged blob should show [stage] action")
	require.Contains(t, view, "error-handling", "pattern title should be rendered")
}

// ---------------------------------------------------------------------------
// TestBlobStaging_StageAction
// ---------------------------------------------------------------------------

// TestBlobStaging_StageAction verifies that pressing Enter on a blob entry
// stages it: badge changes to [staged] and it appears in StagedBlobs.
func TestBlobStaging_StageAction(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoExplore)

	blob := BlobEntry{ID: "t1", Kind: BlobTension, Title: "naming-conv"}
	updated, _ := p.Update(ArchaeoExploreMsg{Entries: []ExploreEntry{
		{Kind: ExploreEntryBlob, Blob: blob},
	}})
	p = updated.(*ArchaeoPane)

	// Cursor is at position 0 (the blob entry). Press Enter to stage.
	p.exploreSel = 0
	updated, _ = p.Update(keyMsg("enter"))
	p = updated.(*ArchaeoPane)

	require.Len(t, p.StagedBlobs(), 1, "one blob should be staged")
	require.Equal(t, "t1", p.StagedBlobs()[0].ID)
	require.True(t, p.exploreEntries[0].IsStaged, "entry should be marked as staged")

	view := p.View()
	require.Contains(t, view, "[staged]", "staged blob should show [staged] badge")
}

// ---------------------------------------------------------------------------
// TestBlobStaging_Unstage
// ---------------------------------------------------------------------------

// TestBlobStaging_Unstage verifies that pressing x on a staged blob removes it
// from StagedBlobs and changes its badge back to [stage].
func TestBlobStaging_Unstage(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoExplore)

	blob := BlobEntry{ID: "t1", Kind: BlobTension, Title: "naming-conv"}
	p.Update(ArchaeoExploreMsg{Entries: []ExploreEntry{{Kind: ExploreEntryBlob, Blob: blob}}}) //nolint
	p.exploreSel = 0
	// Stage it.
	p.Update(keyMsg("enter")) //nolint
	p.exploreEntries[0].IsStaged = true
	p.stagedBlobs = []StagedBlobEntry{{ID: "t1", Kind: BlobTension, Title: "naming-conv"}}

	// Now unstage with x.
	updated, _ := p.Update(keyMsg("x"))
	p = updated.(*ArchaeoPane)

	require.Empty(t, p.StagedBlobs(), "staged blobs should be empty after unstage")
	require.False(t, p.exploreEntries[0].IsStaged, "entry should be unmarked")

	view := p.View()
	require.Contains(t, view, "[stage]", "blob should show [stage] again after unstage")
}

// ---------------------------------------------------------------------------
// TestBlobStaging_PromoteAll
// ---------------------------------------------------------------------------

// TestBlobStaging_PromoteAll verifies that PromoteAll stages all unstaged blob
// proposals in the explore feed.
func TestBlobStaging_PromoteAll(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoExplore)

	entries := []ExploreEntry{
		{Kind: ExploreEntryBlob, Blob: BlobEntry{ID: "t1", Kind: BlobTension, Title: "t1"}},
		{Kind: ExploreEntryText, Text: "some text"},
		{Kind: ExploreEntryBlob, Blob: BlobEntry{ID: "p1", Kind: BlobPattern, Title: "p1"}},
		{Kind: ExploreEntryBlob, Blob: BlobEntry{ID: "l1", Kind: BlobLearning, Title: "l1"}, IsStaged: true},
	}
	updated, _ := p.Update(ArchaeoExploreMsg{Entries: entries})
	p = updated.(*ArchaeoPane)
	// Pre-stage the learning blob so it stays at 1 in the count after PromoteAll.
	p.stagedBlobs = []StagedBlobEntry{{ID: "l1", Kind: BlobLearning, Title: "l1"}}

	p.PromoteAll()

	staged := p.StagedBlobs()
	ids := make([]string, len(staged))
	for i, s := range staged {
		ids[i] = s.ID
	}
	require.Contains(t, ids, "t1", "t1 should be staged")
	require.Contains(t, ids, "p1", "p1 should be staged")
	require.Contains(t, ids, "l1", "l1 should remain staged")
	require.Len(t, staged, 3, "all three blobs should be staged")

	// Verify entries are marked.
	for _, e := range p.exploreEntries {
		if e.Kind == ExploreEntryBlob {
			require.True(t, e.IsStaged, "all blob entries should be marked staged after PromoteAll")
		}
	}
}

// ---------------------------------------------------------------------------
// TestBlobPropagation_ToSidebar
// ---------------------------------------------------------------------------

// TestBlobPropagation_ToSidebar verifies that staged blobs from the explore
// subtab appear as [+] entries in the plan subtab after a subtab switch.
func TestBlobPropagation_ToSidebar(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoExplore)

	// Stage two blobs.
	p.stagedBlobs = []StagedBlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "naming-conv"},
		{ID: "p1", Kind: BlobPattern, Title: "error-handling"},
	}

	// Switch to plan subtab.
	p.SetSubTab(SubTabArchaeoPlan)
	view := p.View()

	require.Contains(t, view, "naming-conv", "staged tension should appear in plan view")
	require.Contains(t, view, "error-handling", "staged pattern should appear in plan view")
	require.Contains(t, view, "[+]", "staged blobs should show [+] badge in plan subtab")
}

// ---------------------------------------------------------------------------
// TestBlobList_Sorting
// ---------------------------------------------------------------------------

// TestBlobList_Sorting verifies that sortBlobEntries returns tensions first,
// patterns second, learning third.
func TestBlobList_Sorting(t *testing.T) {
	blobs := []BlobEntry{
		{ID: "l1", Kind: BlobLearning, Title: "L1"},
		{ID: "p1", Kind: BlobPattern, Title: "P1"},
		{ID: "t1", Kind: BlobTension, Title: "T1"},
		{ID: "t2", Kind: BlobTension, Title: "T2"},
		{ID: "p2", Kind: BlobPattern, Title: "P2"},
	}
	sorted := sortBlobEntries(blobs)
	require.Len(t, sorted, 5)
	require.Equal(t, BlobTension, sorted[0].Kind)
	require.Equal(t, BlobTension, sorted[1].Kind)
	require.Equal(t, BlobPattern, sorted[2].Kind)
	require.Equal(t, BlobPattern, sorted[3].Kind)
	require.Equal(t, BlobLearning, sorted[4].Kind)
}

// TestBlobList_SortingPreservesOrder verifies that within each group the
// original order is preserved.
func TestBlobList_SortingPreservesOrder(t *testing.T) {
	blobs := []BlobEntry{
		{ID: "t1", Kind: BlobTension},
		{ID: "p1", Kind: BlobPattern},
		{ID: "t2", Kind: BlobTension},
		{ID: "p2", Kind: BlobPattern},
	}
	sorted := sortBlobEntries(blobs)
	require.Equal(t, "t1", sorted[0].ID)
	require.Equal(t, "t2", sorted[1].ID)
	require.Equal(t, "p1", sorted[2].ID)
	require.Equal(t, "p2", sorted[3].ID)
}

// ---------------------------------------------------------------------------
// TestBlobList_EmojiRendering
// ---------------------------------------------------------------------------

// TestBlobList_EmojiRendering verifies that BlobKindBadge returns the correct
// emoji for each kind when emoji is enabled.
func TestBlobList_EmojiRendering(t *testing.T) {
	require.Equal(t, "⚡", BlobKindBadge(BlobTension, true))
	require.Equal(t, "🧩", BlobKindBadge(BlobPattern, true))
	require.Equal(t, "💡", BlobKindBadge(BlobLearning, true))
}

// ---------------------------------------------------------------------------
// TestBlobList_FallbackRendering
// ---------------------------------------------------------------------------

// TestBlobList_FallbackRendering verifies that BlobKindBadge returns letter
// badges when emojiEnabled is false.
func TestBlobList_FallbackRendering(t *testing.T) {
	require.Equal(t, "[T]", BlobKindBadge(BlobTension, false))
	require.Equal(t, "[P]", BlobKindBadge(BlobPattern, false))
	require.Equal(t, "[L]", BlobKindBadge(BlobLearning, false))
}

// TestBlobList_EmojiViaPane verifies that SetBlobEmojiEnabled(false) causes
// the plan subtab view to render letter badges instead of emoji.
func TestBlobList_EmojiViaPane(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoPlan)
	p.stagedBlobs = []StagedBlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "naming-conv"},
	}

	p.SetBlobEmojiEnabled(false)
	view := p.View()
	require.Contains(t, view, "[T]", "letter badge should appear when emoji disabled")

	p.SetBlobEmojiEnabled(true)
	view = p.View()
	require.Contains(t, view, "⚡", "emoji badge should appear when emoji enabled")
}

// ---------------------------------------------------------------------------
// TestBlobList_BlankLineSeparation
// ---------------------------------------------------------------------------

// TestBlobList_BlankLineSeparation verifies that renderBlobList inserts a blank
// line between different blob kind groups.
func TestBlobList_BlankLineSeparation(t *testing.T) {
	blobs := []BlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "T1"},
		{ID: "p1", Kind: BlobPattern, Title: "P1"},
	}
	rendered := renderBlobList(blobs, true, 40)

	// Find the tension line and pattern line; expect a blank line between them.
	lines := strings.Split(rendered, "\n")
	tensionIdx := -1
	patternIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "T1") {
			tensionIdx = i
		}
		if strings.Contains(line, "P1") {
			patternIdx = i
		}
	}
	require.True(t, tensionIdx >= 0, "tension line not found")
	require.True(t, patternIdx > tensionIdx, "pattern should come after tension")
	// There should be at least one blank line between them.
	hasBlank := false
	for i := tensionIdx + 1; i < patternIdx; i++ {
		if strings.TrimSpace(lines[i]) == "" {
			hasBlank = true
			break
		}
	}
	require.True(t, hasBlank, "blank line separator expected between tension and pattern groups")
}

// ---------------------------------------------------------------------------
// TestBlobList_NoBlankLineWithinGroup
// ---------------------------------------------------------------------------

// TestBlobList_NoBlankLineWithinGroup verifies that no blank line is inserted
// between items of the same kind group.
func TestBlobList_NoBlankLineWithinGroup(t *testing.T) {
	blobs := []BlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "T1"},
		{ID: "t2", Kind: BlobTension, Title: "T2"},
	}
	rendered := renderBlobList(blobs, true, 40)
	lines := strings.Split(rendered, "\n")
	t1Idx := -1
	t2Idx := -1
	for i, line := range lines {
		if strings.Contains(line, "T1") {
			t1Idx = i
		}
		if strings.Contains(line, "T2") {
			t2Idx = i
		}
	}
	require.True(t, t1Idx >= 0 && t2Idx > t1Idx)
	// Within the tension group, no blank line between consecutive entries.
	for i := t1Idx + 1; i < t2Idx; i++ {
		require.NotEmpty(t, strings.TrimSpace(lines[i]), "no blank line expected within same kind group")
	}
}

// ---------------------------------------------------------------------------
// TestPromoteAll_Command
// ---------------------------------------------------------------------------

// TestPromoteAll_Command verifies that the /promote-all command is registered
// and restricted to the archaeo tab.
func TestPromoteAll_Command(t *testing.T) {
	cmd, ok := rootCommandRegistry.Lookup("promote-all")
	require.True(t, ok, "/promote-all should be registered")
	require.Equal(t, []TabID{TabArchaeo}, cmd.TabFilter)

	// Alias pa should also work.
	cmd, ok = rootCommandRegistry.Lookup("pa")
	require.True(t, ok, "alias pa should be registered")
	require.Equal(t, "promote-all", cmd.Name)
}

// TestPromoteAll_HandlerStagesAllBlobs verifies that rootHandlePromoteAll
// calls PromoteAll on the archaeo pane via the model.
func TestPromoteAll_HandlerStagesAllBlobs(t *testing.T) {
	rt := &fakeRuntimeAdapter{}
	m := newRootModel(rt)
	m.activeTab = TabArchaeo

	// Inject blob entries directly.
	ap := m.archaeo.(*ArchaeoPane)
	ap.SetSubTab(SubTabArchaeoExplore)
	ap.exploreEntries = []ExploreEntry{
		{Kind: ExploreEntryBlob, Blob: BlobEntry{ID: "t1", Kind: BlobTension, Title: "T1"}},
		{Kind: ExploreEntryBlob, Blob: BlobEntry{ID: "p1", Kind: BlobPattern, Title: "P1"}},
	}
	m.archaeo = ap

	newM, _ := rootHandlePromoteAll(&m, nil)
	staged := newM.archaeo.StagedBlobs()
	ids := make([]string, len(staged))
	for i, s := range staged {
		ids[i] = s.ID
	}
	require.Contains(t, ids, "t1")
	require.Contains(t, ids, "p1")
}

// ---------------------------------------------------------------------------
// TestArchaeoPane_CursorNavigation
// ---------------------------------------------------------------------------

// TestArchaeoPane_CursorNavigation verifies that j/k keys move the cursor
// within the explore feed and don't go out of bounds.
func TestArchaeoPane_CursorNavigation(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(100, 30)
	p.SetSubTab(SubTabArchaeoExplore)
	p.exploreEntries = []ExploreEntry{
		{Kind: ExploreEntryBlob, Blob: BlobEntry{ID: "a", Title: "A"}},
		{Kind: ExploreEntryBlob, Blob: BlobEntry{ID: "b", Title: "B"}},
		{Kind: ExploreEntryBlob, Blob: BlobEntry{ID: "c", Title: "C"}},
	}
	p.exploreSel = 0

	updated, _ := p.Update(keyMsg("j"))
	p = updated.(*ArchaeoPane)
	require.Equal(t, 1, p.exploreSel)

	updated, _ = p.Update(keyMsg("j"))
	p = updated.(*ArchaeoPane)
	require.Equal(t, 2, p.exploreSel)

	// At bottom — j should not go further.
	updated, _ = p.Update(keyMsg("j"))
	p = updated.(*ArchaeoPane)
	require.Equal(t, 2, p.exploreSel)

	updated, _ = p.Update(keyMsg("k"))
	p = updated.(*ArchaeoPane)
	require.Equal(t, 1, p.exploreSel)

	// k from 0 should not go below 0.
	p.exploreSel = 0
	updated, _ = p.Update(keyMsg("k"))
	p = updated.(*ArchaeoPane)
	require.Equal(t, 0, p.exploreSel)
}

// ============================================================================
// Phase 5 — Plan subtab: live plan + blob sidebar
// ============================================================================

// ---------------------------------------------------------------------------
// TestPlanSubtab_InternalSplit
// ---------------------------------------------------------------------------

// TestPlanSubtab_InternalSplit verifies that wide terminals (≥ planSidebarCollapseAt)
// render a side-by-side layout with a │ separator column.
func TestPlanSubtab_InternalSplit(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(120, 30) // wide terminal
	p.SetSubTab(SubTabArchaeoPlan)
	p.blobList = []BlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "naming-conv"},
	}

	view := p.View()
	require.Contains(t, view, "│", "wide terminal plan view should contain column separator")
	require.Contains(t, view, "live plan", "main area header should be present")
	require.Contains(t, view, "naming-conv", "sidebar blob should be visible")
}

// ---------------------------------------------------------------------------
// TestPlanSubtab_SidebarCollapseNarrow
// ---------------------------------------------------------------------------

// TestPlanSubtab_SidebarCollapseNarrow verifies that narrow terminals
// (< planSidebarCollapseAt) hide the sidebar by default.
func TestPlanSubtab_SidebarCollapseNarrow(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(80, 30) // narrow
	p.SetSubTab(SubTabArchaeoPlan)
	p.blobList = []BlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "naming-conv"},
	}

	// Default: sidebar hidden → no column separator, blob not in main area.
	view := p.View()
	require.NotContains(t, view, "│", "narrow default should not have column separator")
	require.Contains(t, view, "live plan", "main area should still be present")

	// Toggle sidebar overlay.
	p.sidebarVisible = true
	view = p.View()
	require.Contains(t, view, "naming-conv", "toggled sidebar should show blob list")
}

// ---------------------------------------------------------------------------
// TestPlanSubtab_StepRendering
// ---------------------------------------------------------------------------

// TestPlanSubtab_StepRendering verifies that plan steps are rendered with
// the correct status icons (✓ / ▶ / · / ✗) and linked blob titles.
func TestPlanSubtab_StepRendering(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(120, 30)
	p.SetSubTab(SubTabArchaeoPlan)

	p.livePlan = &ActivePlanView{
		Title: "my plan",
		Steps: []PlanStepInfo{
			{ID: "s1", Title: "explore structure", Status: "done"},
			{ID: "s2", Title: "resolve naming tension", Status: "running"},
			{ID: "s3", Title: "verify anchors", Status: "pending"},
			{ID: "s4", Title: "validate output", Status: "failed"},
		},
	}
	// Link a blob to step s2.
	p.blobList = []BlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "naming-conv", InPlan: true, StepID: "s2"},
	}

	view := p.View()
	require.Contains(t, view, "✓", "done step should show ✓")
	require.Contains(t, view, "▶", "running step should show ▶")
	require.Contains(t, view, "·", "pending step should show ·")
	require.Contains(t, view, "✗", "failed step should show ✗")
	require.Contains(t, view, "naming-conv", "linked blob title should appear next to step")
}

// TestStepStatusIcon verifies the stepStatusIcon utility.
func TestStepStatusIcon(t *testing.T) {
	require.Equal(t, "✓", stepStatusIcon("done"))
	require.Equal(t, "▶", stepStatusIcon("running"))
	require.Equal(t, "·", stepStatusIcon("pending"))
	require.Equal(t, "·", stepStatusIcon("ready"))
	require.Equal(t, "✗", stepStatusIcon("failed"))
	require.Equal(t, "✗", stepStatusIcon("blocked"))
}

// ---------------------------------------------------------------------------
// TestPlanSubtab_TabFocusToggle
// ---------------------------------------------------------------------------

// TestPlanSubtab_TabFocusToggle verifies that the Tab key toggles focus between
// the main area and the blob sidebar.
func TestPlanSubtab_TabFocusToggle(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(120, 30)
	p.SetSubTab(SubTabArchaeoPlan)
	require.False(t, p.planFocused, "initial focus should be main area")

	// Press Tab — moves focus to sidebar.
	updated, _ := p.Update(tabKeyMsg())
	p = updated.(*ArchaeoPane)
	require.True(t, p.planFocused, "Tab should move focus to sidebar")

	// Press Tab again — moves focus back to main area.
	updated, _ = p.Update(tabKeyMsg())
	p = updated.(*ArchaeoPane)
	require.False(t, p.planFocused, "second Tab should return focus to main area")
}

// ---------------------------------------------------------------------------
// TestBlobSidebar_AddBlob
// ---------------------------------------------------------------------------

// TestBlobSidebar_AddBlob verifies that pressing Enter on a [+] blob in the
// sidebar dispatches AddBlobToPlan via the runtime adapter.
func TestBlobSidebar_AddBlob(t *testing.T) {
	var addedBlobID string
	var addedWorkflowID string
	rt := &recordingAdapter{}

	// Use a custom adapter that records AddBlobToPlan.
	p := NewArchaeoPane(rt)
	p.SetSize(120, 30)
	p.SetSubTab(SubTabArchaeoPlan)
	p.planFocused = true // focus on sidebar

	p.blobList = []BlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "naming-conv", InPlan: false},
	}
	p.blobSel = 0

	_, cmd := p.Update(keyMsg("enter"))
	require.NotNil(t, cmd, "Enter on [+] blob should return a command")

	// Execute the command and verify it produces a blobAddedMsg.
	msg := cmd()
	added, ok := msg.(blobAddedMsg)
	require.True(t, ok, "command should produce blobAddedMsg, got %T", msg)
	_ = addedBlobID
	_ = addedWorkflowID
	_ = added
	require.Equal(t, "t1", added.blobID)
}

// ---------------------------------------------------------------------------
// TestBlobSidebar_RemoveBlob
// ---------------------------------------------------------------------------

// TestBlobSidebar_RemoveBlob verifies that pressing x on an [in] blob in the
// sidebar dispatches RemoveBlobFromPlan via the runtime adapter.
func TestBlobSidebar_RemoveBlob(t *testing.T) {
	rt := &recordingAdapter{}
	p := NewArchaeoPane(rt)
	p.SetSize(120, 30)
	p.SetSubTab(SubTabArchaeoPlan)
	p.planFocused = true

	p.blobList = []BlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "naming-conv", InPlan: true, StepID: "s1"},
	}
	p.blobSel = 0

	_, cmd := p.Update(keyMsg("x"))
	require.NotNil(t, cmd, "x on [in] blob should return a command")

	msg := cmd()
	removed, ok := msg.(blobRemovedMsg)
	require.True(t, ok, "command should produce blobRemovedMsg, got %T", msg)
	require.Equal(t, "t1", removed.blobID)
}

// TestBlobSidebar_EnterOnInPlanBlob verifies that pressing Enter on an [in]
// blob (already in plan) does not dispatch AddBlobToPlan.
func TestBlobSidebar_EnterOnInPlanBlob(t *testing.T) {
	rt := &recordingAdapter{}
	p := NewArchaeoPane(rt)
	p.SetSize(120, 30)
	p.SetSubTab(SubTabArchaeoPlan)
	p.planFocused = true

	p.blobList = []BlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "naming-conv", InPlan: true, StepID: "s1"},
	}
	p.blobSel = 0

	_, cmd := p.Update(keyMsg("enter"))
	require.Nil(t, cmd, "Enter on [in] blob should not dispatch anything")
}

// ---------------------------------------------------------------------------
// TestBlobSidebar_ExpandDetail
// ---------------------------------------------------------------------------

// TestBlobSidebar_ExpandDetail verifies that pressing e on a focused blob
// expands its description and anchor refs inline in the sidebar.
func TestBlobSidebar_ExpandDetail(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(120, 40)
	p.SetSubTab(SubTabArchaeoPlan)
	p.planFocused = true

	p.blobList = []BlobEntry{
		{
			ID:          "t1",
			Kind:        BlobTension,
			Title:       "naming-conv",
			Description: "Inconsistent naming between registry and provider interfaces.",
			AnchorRefs:  []string{"framework/capability/registry.go:42"},
		},
	}
	p.blobSel = 0

	// Press e to expand.
	updated, _ := p.Update(keyMsg("e"))
	p = updated.(*ArchaeoPane)
	require.Equal(t, "t1", p.expandedBlobID, "expanded blob ID should be set")

	view := p.viewPlanSidebar(60) // wide enough to not truncate the anchor ref
	require.Contains(t, view, "Inconsistent naming", "description should appear when expanded")
	require.Contains(t, view, "registry.go", "anchor ref should appear when expanded")

	// Press e again to collapse.
	updated, _ = p.Update(keyMsg("e"))
	p = updated.(*ArchaeoPane)
	require.Equal(t, "", p.expandedBlobID, "expanded blob ID should be cleared on second e")
}

// ---------------------------------------------------------------------------
// TestBlobSidebar_Scroll
// ---------------------------------------------------------------------------

// TestBlobSidebar_Scroll verifies that the blob cursor wraps correctly with
// j/k and that blobScrollOff tracks the viewport.
func TestBlobSidebar_Scroll(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(120, 10) // small height to force scrolling
	p.SetSubTab(SubTabArchaeoPlan)
	p.planFocused = true

	// Fill the blob list beyond the visible rows.
	var blobs []BlobEntry
	for i := 0; i < 20; i++ {
		blobs = append(blobs, BlobEntry{
			ID:   fmt.Sprintf("t%d", i),
			Kind: BlobTension,
			Title: fmt.Sprintf("tension-%d", i),
		})
	}
	p.blobList = blobs
	p.blobSel = 0
	p.blobScrollOff = 0

	// Move down past the visible window to trigger scrolling.
	rows := visibleBlobRows(p.height)
	for i := 0; i < rows+2; i++ {
		updated, _ := p.Update(keyMsg("j"))
		p = updated.(*ArchaeoPane)
	}
	require.Greater(t, p.blobScrollOff, 0, "scrolling down should increase blobScrollOff")

	// Moving back up should decrease the offset.
	for i := 0; i < 2; i++ {
		updated, _ := p.Update(keyMsg("k"))
		p = updated.(*ArchaeoPane)
	}
	require.Less(t, p.blobScrollOff, rows+2, "scrolling up should reduce blobScrollOff")
}

// ---------------------------------------------------------------------------
// TestPlanPolling_UpdatesView
// ---------------------------------------------------------------------------

// TestPlanPolling_UpdatesView verifies that a PlanUpdatedMsg delivered to the
// pane updates the plan and re-renders with the new steps.
func TestPlanPolling_UpdatesView(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(120, 30)
	p.SetSubTab(SubTabArchaeoPlan)

	updated, _ := p.Update(PlanUpdatedMsg{
		Plan: &ActivePlanView{
			Title: "polled plan",
			Steps: []PlanStepInfo{
				{ID: "s1", Title: "first step", Status: "pending"},
			},
		},
	})
	p = updated.(*ArchaeoPane)
	require.NotNil(t, p.livePlan)
	require.Equal(t, "polled plan", p.livePlan.Title)

	view := p.View()
	require.Contains(t, view, "first step", "polled plan step should appear in view")
}

// ---------------------------------------------------------------------------
// TestBlobPolling_UpdatesSidebar
// ---------------------------------------------------------------------------

// TestBlobPolling_UpdatesSidebar verifies that a BlobsUpdatedMsg delivered to
// the pane sorts and renders the new blob list in the sidebar.
func TestBlobPolling_UpdatesSidebar(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(120, 30)
	p.SetSubTab(SubTabArchaeoPlan)

	updated, _ := p.Update(BlobsUpdatedMsg{
		Blobs: []BlobEntry{
			{ID: "l1", Kind: BlobLearning, Title: "prefer-iface"},
			{ID: "t1", Kind: BlobTension, Title: "naming-conv"},
			{ID: "p1", Kind: BlobPattern, Title: "error-handling"},
		},
	})
	p = updated.(*ArchaeoPane)
	require.Len(t, p.blobList, 3)
	// Verify sorted: tensions first.
	require.Equal(t, BlobTension, p.blobList[0].Kind)
	require.Equal(t, BlobPattern, p.blobList[1].Kind)
	require.Equal(t, BlobLearning, p.blobList[2].Kind)

	view := p.View()
	require.Contains(t, view, "naming-conv")
	require.Contains(t, view, "prefer-iface")
}

// ---------------------------------------------------------------------------
// TestAddBlobHighlight
// ---------------------------------------------------------------------------

// TestAddBlobHighlight verifies that when AddBlobToPlan succeeds and a
// PlanUpdatedMsg arrives with a new step, that step is briefly highlighted.
func TestAddBlobHighlight(t *testing.T) {
	rt := &recordingAdapter{}
	p := NewArchaeoPane(rt)
	p.SetSize(120, 30)
	p.SetSubTab(SubTabArchaeoPlan)
	p.planFocused = true

	p.blobList = []BlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "naming-conv", InPlan: false},
	}
	p.blobSel = 0

	// Dispatch AddBlobToPlan command, then simulate success response.
	_, cmd := p.Update(keyMsg("enter"))
	require.NotNil(t, cmd)

	// Simulate blobAddedMsg success — sets newStepPending.
	updated, _ := p.Update(blobAddedMsg{blobID: "t1", err: nil})
	p = updated.(*ArchaeoPane)
	require.True(t, p.newStepPending, "newStepPending should be set after successful add")

	// Deliver PlanUpdatedMsg with a new step — triggers highlight.
	updated, clearCmd := p.Update(PlanUpdatedMsg{
		Plan: &ActivePlanView{
			Steps: []PlanStepInfo{
				{ID: "s-new", Title: "resolve naming tension", Status: "pending"},
			},
		},
	})
	p = updated.(*ArchaeoPane)
	require.False(t, p.newStepPending, "newStepPending should be cleared after PlanUpdatedMsg")
	require.Equal(t, "s-new", p.newlyAddedStepID, "new step should be highlighted")
	require.NotNil(t, clearCmd, "clearCmd should be returned to clear highlight")

	// Verify the highlighted step appears differently in the view.
	view := p.viewPlanMain(80)
	require.Contains(t, view, "resolve naming tension", "new step should appear in view")

	// Deliver clearPlanHighlightMsg — clears highlight.
	updated, _ = p.Update(clearPlanHighlightMsg{})
	p = updated.(*ArchaeoPane)
	require.Equal(t, "", p.newlyAddedStepID, "highlight should be cleared")
}

// ---------------------------------------------------------------------------
// TestEffectiveBlobList
// ---------------------------------------------------------------------------

// TestEffectiveBlobList verifies that effectiveBlobList merges blobList and
// stagedBlobs without duplicates, and sorts the result.
func TestEffectiveBlobList(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.blobList = []BlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "from-backend", InPlan: true},
	}
	p.stagedBlobs = []StagedBlobEntry{
		{ID: "t1", Kind: BlobTension, Title: "from-backend"},  // duplicate — should not appear twice
		{ID: "p1", Kind: BlobPattern, Title: "staged-pattern"}, // new staged blob
	}

	blobs := p.effectiveBlobList()
	require.Len(t, blobs, 2, "duplicate staged blob should not appear twice")
	ids := make([]string, len(blobs))
	for i, b := range blobs {
		ids[i] = b.ID
	}
	require.Contains(t, ids, "t1")
	require.Contains(t, ids, "p1")
	// Backend version should take precedence (InPlan=true).
	for _, b := range blobs {
		if b.ID == "t1" {
			require.True(t, b.InPlan, "backend version should take precedence")
		}
	}
}

// ---------------------------------------------------------------------------
// TestPlanOutputLines
// ---------------------------------------------------------------------------

// TestPlanOutputLines verifies that blobAddedMsg and blobRemovedMsg append to
// planOutputLines, and that the output stream appears in the plan main view.
func TestPlanOutputLines(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(120, 30)
	p.SetSubTab(SubTabArchaeoPlan)

	updated, _ := p.Update(blobAddedMsg{blobID: "t1", err: nil})
	p = updated.(*ArchaeoPane)
	require.NotEmpty(t, p.planOutputLines)

	updated, _ = p.Update(blobRemovedMsg{blobID: "t1", err: nil})
	p = updated.(*ArchaeoPane)
	require.Len(t, p.planOutputLines, 2)

	view := p.viewPlanMain(80)
	require.Contains(t, view, "blob added to plan")
	require.Contains(t, view, "blob removed from plan")
}

// TestPlanOutputLines_Error verifies error messages are recorded.
func TestPlanOutputLines_Error(t *testing.T) {
	p := NewArchaeoPane(nil)
	p.SetSize(120, 30)

	updated, _ := p.Update(blobAddedMsg{blobID: "t1", err: fmt.Errorf("permission denied")})
	p = updated.(*ArchaeoPane)
	require.Contains(t, p.planOutputLines[0], "permission denied")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// keyMsg builds a tea.KeyMsg for use in tests.
func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// tabKeyMsg builds a Tab key message.
func tabKeyMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyTab}
}
