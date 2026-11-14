package tui

import "testing"


func TestV2DecisionFocusSurvivesLegacyApprovalSnapshots(t *testing.T) {
	m, _ := newFocusTestModel()
	m.notifQ.Push(NotificationItem{ID: "hitl-1", Kind: NotifKindHITL, Msg: "execution approval: euclo.policy.gate"})
	m.decisionFocused = true

	// Live snapshots arrive continuously while a legacy permission approval is
	// pending in the notification queue. None of them carries a conversation
	// decision, and none of them may discard the deliberate focus.
	m.applyFrontend(FrontendSnapshot{Revision: 1})
	if !m.decisionFocused {
		t.Fatal("legacy approval focus discarded by a decision-less snapshot")
	}

	// An open conversation decision that disappears does close its detail.
	m.applyFrontend(FrontendSnapshot{Revision: 2, Decision: &FrontendDecision{ID: "d1", Question: "Proceed?"}})
	m.decisionFocused = true
	m.applyFrontend(FrontendSnapshot{Revision: 3})
	if m.decisionFocused {
		t.Fatal("expired conversation decision left the decision detail focused")
	}
}
