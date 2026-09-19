package store

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/johnspence0212/NonLinear/internal/model"
)

func TestVersionedProjectRoundTrip(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject(CreateProject{Title: "Tracker", Destination: "Ship NonLinear"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Identifier != "P-1" || p.Stage != model.StageWayfinding {
		t.Fatalf("project: %+v", p)
	}
	m, err := s.Create(CreateIssue{Title: "Decision map", Labels: []string{"wayfinder:map"}, Body: "## Destination\n\nShip NonLinear.\n"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != model.KindDecisionMap || m.ProjectID == nil {
		t.Fatalf("map: %+v", m)
	}
	if _, err := s.ReadyForSpec(m.ID); err != nil {
		t.Fatal(err)
	}
	spec, err := s.CreateSpec(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Kind != model.KindSpec || spec.Lifecycle != model.SpecLifecycleDraft || spec.DerivedFromArtifactID == nil || *spec.DerivedFromArtifactID != m.ID {
		t.Fatalf("spec: %+v", spec)
	}
	if _, err := s.ApproveSpec(spec.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := s.CreatePlan(spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != model.KindPlan || plan.Lifecycle != model.PlanLifecycleDraft || plan.DerivedFromArtifactID == nil || *plan.DerivedFromArtifactID != spec.ID {
		t.Fatalf("plan: %+v", plan)
	}

	raw, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	var disk model.DB
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if disk.SchemaVersion != model.SchemaVersion {
		t.Fatalf("schemaVersion %d", disk.SchemaVersion)
	}
	if len(disk.Projects) == 0 {
		t.Fatal("expected projects on disk")
	}

	reopened, err := Open(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != model.KindPlan || got.DerivedFromArtifactID == nil || *got.DerivedFromArtifactID != spec.ID {
		t.Fatalf("reopened plan: %+v", got)
	}
}

func TestLifecycleTransitions(t *testing.T) {
	s := testStore(t)
	m, err := s.Create(CreateIssue{Title: "Map", Labels: []string{"wayfinder:map"}, Body: "## Destination\n\nDone.\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSpec(m.ID); err == nil {
		t.Fatal("create spec before ready-for-spec")
	}
	ready, err := s.ReadyForSpec(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Lifecycle != model.MapLifecycleReadyForSpec {
		t.Fatalf("ready: %s", ready.Lifecycle)
	}
	spec, err := s.CreateSpec(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if spec.DerivedFromArtifactID == nil || *spec.DerivedFromArtifactID != m.ID {
		t.Fatalf("spec derivedFrom: %+v", spec.DerivedFromArtifactID)
	}
	if _, err := s.CreatePlan(spec.ID); err == nil {
		t.Fatal("create plan before approve")
	}
	approved, err := s.ApproveSpec(spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Lifecycle != model.SpecLifecycleApproved {
		t.Fatalf("approve: %s", approved.Lifecycle)
	}
	plan, err := s.CreatePlan(spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.DerivedFromArtifactID == nil || *plan.DerivedFromArtifactID != spec.ID {
		t.Fatalf("plan derivedFrom: %+v", plan.DerivedFromArtifactID)
	}
	if _, err := s.CreatePlan(spec.ID); err == nil {
		t.Fatal("duplicate plan")
	}
	if _, err := s.CreateSpec(m.ID); err == nil {
		t.Fatal("duplicate spec")
	}
}

func TestDerivedStageTable(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Store) int
		want  string
	}{
		{
			name: "wayfinding",
			setup: func(s *Store) int {
				m, err := s.Create(CreateIssue{Title: "Map", Labels: []string{"wayfinder:map"}})
				if err != nil {
					t.Fatal(err)
				}
				return *m.ProjectID
			},
			want: model.StageWayfinding,
		},
		{
			name: "ready_for_spec",
			setup: func(s *Store) int {
				m, err := s.Create(CreateIssue{Title: "Map", Labels: []string{"wayfinder:map"}})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.ReadyForSpec(m.ID); err != nil {
					t.Fatal(err)
				}
				return *m.ProjectID
			},
			want: model.StageReadyForSpec,
		},
		{
			name: "spec_review",
			setup: func(s *Store) int {
				m := mustMapReady(t, s)
				if _, err := s.CreateSpec(m.ID); err != nil {
					t.Fatal(err)
				}
				return *m.ProjectID
			},
			want: model.StageSpecReview,
		},
		{
			name: "ready_for_tickets no plan",
			setup: func(s *Store) int {
				m := mustMapReady(t, s)
				spec, err := s.CreateSpec(m.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.ApproveSpec(spec.ID); err != nil {
					t.Fatal(err)
				}
				return *m.ProjectID
			},
			want: model.StageReadyForTickets,
		},
		{
			name: "ready_for_tickets plan draft",
			setup: func(s *Store) int {
				m := mustMapReady(t, s)
				spec, err := s.CreateSpec(m.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.ApproveSpec(spec.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := s.CreatePlan(spec.ID); err != nil {
					t.Fatal(err)
				}
				return *m.ProjectID
			},
			want: model.StageReadyForTickets,
		},
		{
			name: "implementing",
			setup: func(s *Store) int {
				plan := mustPlan(t, s)
				if _, err := s.ActivatePlan(plan.ID); err != nil {
					t.Fatal(err)
				}
				return *plan.ProjectID
			},
			want: model.StageImplementing,
		},
		{
			name: "complete",
			setup: func(s *Store) int {
				plan := mustPlan(t, s)
				if _, err := s.ActivatePlan(plan.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := s.DeliverPlan(plan.ID); err != nil {
					t.Fatal(err)
				}
				return *plan.ProjectID
			},
			want: model.StageComplete,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			pid := tc.setup(s)
			got, err := s.GetProject(pid)
			if err != nil {
				t.Fatal(err)
			}
			if got.Stage != tc.want {
				t.Fatalf("stage %s, want %s", got.Stage, tc.want)
			}
		})
	}
}

func TestFrontierExcludesSpecAndPlan(t *testing.T) {
	s := testStore(t)
	m := mustMapReady(t, s)
	spec, err := s.CreateSpec(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApproveSpec(spec.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := s.CreatePlan(spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := s.Create(CreateIssue{Title: "Implement", ParentID: &plan.ID, Labels: []string{"ready-for-agent"}})
	if err != nil {
		t.Fatal(err)
	}
	front := s.Frontier(nil)
	if len(front) != 1 || front[0].ID != ticket.ID {
		t.Fatalf("frontier: %+v", ids(front))
	}
	gotSpec, err := s.Get(spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotSpec.Frontier {
		t.Fatal("spec should not be frontier")
	}
}

func mustMapReady(t *testing.T, s *Store) model.IssueView {
	t.Helper()
	m, err := s.Create(CreateIssue{Title: "Map", Labels: []string{"wayfinder:map"}, Body: "## Destination\n\nShip it.\n"})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := s.ReadyForSpec(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func mustPlan(t *testing.T, s *Store) model.IssueView {
	t.Helper()
	m := mustMapReady(t, s)
	spec, err := s.CreateSpec(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApproveSpec(spec.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := s.CreatePlan(spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestMoveIssueToProjectMovesDescendants(t *testing.T) {
	s := testStore(t)
	m, err := s.Create(CreateIssue{Title: "Solo loop", Labels: []string{"wayfinder:map"}})
	if err != nil {
		t.Fatal(err)
	}
	parent := m.ID
	child, err := s.Create(CreateIssue{Title: "Lock class", ParentID: &parent})
	if err != nil {
		t.Fatal(err)
	}
	dest, err := s.CreateProject(CreateProject{Title: "Idle Frontier"})
	if err != nil {
		t.Fatal(err)
	}
	srcID := *m.ProjectID
	if srcID == dest.ID {
		t.Fatal("expected implicit project distinct from dest")
	}
	got, err := s.MoveToProject(MoveToProject{ID: &m.ID, ProjectID: dest.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Moved) != 2 {
		t.Fatalf("moved %v", got.Moved)
	}
	for _, id := range []int{m.ID, child.ID} {
		issue, err := s.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if issue.ProjectID == nil || *issue.ProjectID != dest.ID {
			t.Fatalf("issue %d projectId %v want %d", id, issue.ProjectID, dest.ID)
		}
	}
	src, err := s.GetProject(srcID)
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Maps) != 0 {
		t.Fatalf("source still has maps: %+v", src.Maps)
	}
}

func TestMoveAllIssuesFromProject(t *testing.T) {
	s := testStore(t)
	m := mustMapReady(t, s)
	spec, err := s.CreateSpec(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	from := *m.ProjectID
	dest, err := s.CreateProject(CreateProject{Title: "Home"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.MoveToProject(MoveToProject{FromProjectID: &from, ProjectID: dest.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Moved) != 2 {
		t.Fatalf("moved %v", got.Moved)
	}
	for _, id := range []int{m.ID, spec.ID} {
		issue, err := s.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if issue.ProjectID == nil || *issue.ProjectID != dest.ID {
			t.Fatalf("issue %d projectId %v want %d", id, issue.ProjectID, dest.ID)
		}
	}
}

func TestDeleteProjectRemovesIssues(t *testing.T) {
	s := testStore(t)
	m, err := s.Create(CreateIssue{Title: "Solo loop", Labels: []string{"wayfinder:map"}})
	if err != nil {
		t.Fatal(err)
	}
	parent := m.ID
	if _, err := s.Create(CreateIssue{Title: "Lock class", ParentID: &parent}); err != nil {
		t.Fatal(err)
	}
	pid := *m.ProjectID
	got, err := s.DeleteProject(pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Deleted) != 2 {
		t.Fatalf("deleted %v", got.Deleted)
	}
	if _, err := s.GetProject(pid); err == nil {
		t.Fatal("expected project gone")
	}
	if _, err := s.Get(m.ID); err == nil {
		t.Fatal("expected map gone")
	}
}

func TestDeleteEmptyProject(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject(CreateProject{Title: "Spare"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.DeleteProject(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Deleted) != 0 {
		t.Fatalf("deleted %v", got.Deleted)
	}
	if _, err := s.GetProject(p.ID); err == nil {
		t.Fatal("expected project gone")
	}
}

