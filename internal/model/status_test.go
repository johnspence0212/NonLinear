package model

import (
	"testing"
	"time"
)

func TestBuildProjectStatusFrontierAndCounts(t *testing.T) {
	pid := 1
	parent := 2
	blocker := 3
	now := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	assignee := "cursor"
	issues := []Issue{
		{
			ID: 2, Identifier: "NL-2", Title: "Chart the destination",
			State: StateOpen, Labels: []string{"wayfinder:map"}, Kind: KindDecisionMap,
			Lifecycle: MapLifecycleActive, ProjectID: &pid,
		},
		{
			ID: 3, Identifier: "NL-3", Title: "What store?",
			State: StateOpen, Labels: []string{"wayfinder:grilling"},
			ParentID: &parent, ProjectID: &pid,
		},
		{
			ID: 4, Identifier: "NL-4", Title: "How to expose MCP?",
			State: StateOpen, Labels: []string{"wayfinder:grilling"},
			ParentID: &parent, BlockedBy: []int{blocker}, ProjectID: &pid,
		},
		{
			ID: 5, Identifier: "NL-5", Title: "Claimed work",
			State: StateOpen, Labels: []string{"wayfinder:task"},
			ParentID: &parent, Assignee: &assignee, ProjectID: &pid,
		},
		{
			ID: 6, Identifier: "NL-6", Title: "Already answered",
			State: StateClosed, Labels: []string{"wayfinder:research"},
			ParentID: &parent, ProjectID: &pid,
		},
	}
	byID := map[int]Issue{}
	for _, issue := range issues {
		byID[issue.ID] = issue
	}
	status := BuildProjectStatus(Project{
		ID: 1, Identifier: "P-1", Title: "Tracker", Destination: "Ship it.",
		UpdatedAt: now,
	}, issues, byID)

	if status.Kind != ProjectStatusKind || status.Stage != StageWayfinding {
		t.Fatalf("header: %+v", status)
	}
	if status.Progress.Maps != 1 || status.Progress.Tickets.Total != 4 {
		t.Fatalf("progress: %+v", status.Progress)
	}
	if status.Progress.Tickets.Open != 3 || status.Progress.Tickets.Closed != 1 {
		t.Fatalf("open/closed: %+v", status.Progress.Tickets)
	}
	if status.Progress.Tickets.Frontier != 1 || status.Progress.Tickets.Claimed != 1 || status.Progress.Tickets.Blocked != 1 {
		t.Fatalf("split: %+v", status.Progress.Tickets)
	}
	if len(status.Frontier) != 1 || status.Frontier[0].ID != 3 {
		t.Fatalf("frontier: %+v", status.Frontier)
	}
	if status.Next == nil || status.Next.ID != 3 {
		t.Fatalf("next: %+v", status.Next)
	}
	if status.NextAction.Tool != "claim_issue" || status.NextAction.ID == nil || *status.NextAction.ID != 3 {
		t.Fatalf("nextAction: %+v", status.NextAction)
	}
	if len(status.Maps) != 1 || status.Maps[0].Tickets.Total != 4 || status.Maps[0].Tickets.Frontier != 1 {
		t.Fatalf("map tickets: %+v", status.Maps)
	}
	if status.Destination != "Ship it." {
		t.Fatalf("destination: %q", status.Destination)
	}
}

func TestDeriveNextActionStages(t *testing.T) {
	id := 9
	cases := []struct {
		name string
		in   ProjectStatus
		tool string
	}{
		{
			name: "empty project",
			in:   ProjectStatus{Stage: StageWayfinding},
			tool: "create_issue",
		},
		{
			name: "ready for spec",
			in: ProjectStatus{
				Stage: StageReadyForSpec,
				Maps:  []StatusArtifact{{ID: id, Identifier: "NL-9", Lifecycle: MapLifecycleReadyForSpec}},
			},
			tool: "advance_to_spec",
		},
		{
			name: "spec review",
			in: ProjectStatus{
				Stage: StageSpecReview,
				Specs: []StatusArtifact{{ID: id, Identifier: "NL-9", Lifecycle: SpecLifecycleDraft}},
			},
			tool: "approve_spec",
		},
		{
			name: "approved spec",
			in: ProjectStatus{
				Stage: StageReadyForTickets,
				Specs: []StatusArtifact{{ID: id, Identifier: "NL-9", Lifecycle: SpecLifecycleApproved}},
			},
			tool: "create_plan",
		},
		{
			name: "draft plan",
			in: ProjectStatus{
				Stage: StageReadyForTickets,
				Plans: []StatusArtifact{{ID: id, Identifier: "NL-9", Lifecycle: PlanLifecycleDraft}},
			},
			tool: "activate_plan",
		},
		{
			name: "active plan no open tickets",
			in: ProjectStatus{
				Stage: StageImplementing,
				Plans: []StatusArtifact{{ID: id, Identifier: "NL-9", Lifecycle: PlanLifecycleActive}},
			},
			tool: "deliver_plan",
		},
		{
			name: "complete",
			in:   ProjectStatus{Stage: StageComplete},
			tool: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveNextAction(tc.in)
			if got.Tool != tc.tool {
				t.Fatalf("tool %q want %q (%s)", got.Tool, tc.tool, got.Reason)
			}
		})
	}
}
