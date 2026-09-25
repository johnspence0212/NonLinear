package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/johnspence0212/NonLinear/internal/model"
)

func TestProjectStatusLookupAndCounts(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject(CreateProject{Title: "Idle Frontier", Destination: "A local tracker."})
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Create(CreateIssue{Title: "Chart the destination", Labels: []string{"wayfinder:map"}, ProjectID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	parent := m.ID
	a, err := s.Create(CreateIssue{Title: "What store?", Labels: []string{"wayfinder:grilling"}, ParentID: &parent})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(CreateIssue{Title: "How to expose MCP?", Labels: []string{"wayfinder:grilling"}, ParentID: &parent})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBlockedBy(b.ID, []int{a.ID}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ProjectStatus(&p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != model.ProjectStatusKind || got.Identifier != "P-1" || got.Stage != model.StageWayfinding {
		t.Fatalf("status: %+v", got)
	}
	if got.Progress.Tickets.Total != 2 || got.Progress.Tickets.Frontier != 1 || got.Next == nil || got.Next.ID != a.ID {
		t.Fatalf("tickets: %+v next=%+v", got.Progress.Tickets, got.Next)
	}

	byIdent, err := s.ProjectStatus(nil, "P-1")
	if err != nil {
		t.Fatal(err)
	}
	if byIdent.ID != p.ID {
		t.Fatalf("P-1: %+v", byIdent)
	}
	byTitle, err := s.ProjectStatus(nil, "idle")
	if err != nil {
		t.Fatal(err)
	}
	if byTitle.ID != p.ID {
		t.Fatalf("title: %+v", byTitle)
	}

	if _, err := s.CreateProject(CreateProject{Title: "Idle Hands"}); err != nil {
		t.Fatal(err)
	}
	_, err = s.ProjectStatus(nil, "idle")
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "P-1") || !strings.Contains(err.Error(), "P-2") {
		t.Fatalf("ambiguous: %v", err)
	}
	if _, err := s.ProjectStatus(nil, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := s.ProjectStatus(nil, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty: %v", err)
	}
}

func TestProjectStatusP8(t *testing.T) {
	s := testStore(t)
	var p8 model.ProjectView
	for i := 1; i <= 8; i++ {
		p, err := s.CreateProject(CreateProject{Title: fmt.Sprintf("Project %d", i)})
		if err != nil {
			t.Fatal(err)
		}
		p8 = p
	}
	if p8.Identifier != "P-8" {
		t.Fatalf("identifier %s", p8.Identifier)
	}
	got, err := s.ProjectStatus(nil, "P-8")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != p8.ID || got.Identifier != "P-8" || got.Title != "Project 8" {
		t.Fatalf("P-8: %+v", got)
	}
	lower, err := s.ProjectStatus(nil, "p-8")
	if err != nil || lower.ID != p8.ID {
		t.Fatalf("p-8: %+v %v", lower, err)
	}
}

func TestProjectStatusLifecycleNextAction(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject(CreateProject{Title: "Ship"})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.ProjectStatus(&p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if empty.NextAction.Tool != "create_issue" {
		t.Fatalf("empty: %+v", empty.NextAction)
	}

	m, err := s.Create(CreateIssue{
		Title: "Map", Labels: []string{"wayfinder:map"},
		Body: "## Destination\n\nShip.\n", ProjectID: &p.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadyForSpec(m.ID); err != nil {
		t.Fatal(err)
	}
	ready, err := s.ProjectStatus(nil, "ship")
	if err != nil {
		t.Fatal(err)
	}
	if ready.Stage != model.StageReadyForSpec || ready.NextAction.Tool != "advance_to_spec" {
		t.Fatalf("ready: stage=%s action=%+v", ready.Stage, ready.NextAction)
	}

	spec, err := s.CreateSpec(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	review, err := s.ProjectStatus(&p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if review.Stage != model.StageSpecReview || review.NextAction.Tool != "approve_spec" {
		t.Fatalf("review: %+v", review.NextAction)
	}

	if _, err := s.ApproveSpec(spec.ID); err != nil {
		t.Fatal(err)
	}
	approved, err := s.ProjectStatus(&p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Stage != model.StageReadyForTickets || approved.NextAction.Tool != "create_plan" {
		t.Fatalf("approved: %+v", approved.NextAction)
	}

	plan, err := s.CreatePlan(spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.ProjectStatus(&p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if draft.NextAction.Tool != "activate_plan" || draft.NextAction.ID == nil || *draft.NextAction.ID != plan.ID {
		t.Fatalf("draft plan: %+v", draft.NextAction)
	}
	if _, err := s.ActivatePlan(plan.ID); err != nil {
		t.Fatal(err)
	}
	ticket, err := s.Create(CreateIssue{Title: "Implement", ParentID: &plan.ID})
	if err != nil {
		t.Fatal(err)
	}
	active, err := s.ProjectStatus(&p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if active.Stage != model.StageImplementing || active.Next == nil || active.Next.ID != ticket.ID {
		t.Fatalf("active: %+v", active)
	}
	if active.Plans[0].Tickets.Open != 1 {
		t.Fatalf("plan tickets: %+v", active.Plans[0].Tickets)
	}

	closed := model.StateClosed
	if _, err := s.Update(ticket.ID, UpdateIssue{State: &closed}); err != nil {
		t.Fatal(err)
	}
	done, err := s.ProjectStatus(&p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if done.NextAction.Tool != "deliver_plan" {
		t.Fatalf("deliver: %+v", done.NextAction)
	}
}
