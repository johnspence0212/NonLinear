package store

import (
	"testing"

	"github.com/johnspence0212/NonLinear/internal/model"
)

func TestCreateAndListBugs(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject(CreateProject{Title: "Tracker"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(CreateIssue{Title: "Map", Labels: []string{"wayfinder:map"}, ProjectID: &p.ID}); err != nil {
		t.Fatal(err)
	}

	first, err := s.CreateBug(CreateBug{Title: "Overflow on mobile", Body: "The row wraps.", ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if first.Identifier != "B-1" || first.Kind != model.KindBug || first.ProjectID == nil || *first.ProjectID != p.ID {
		t.Fatalf("first: %+v", first)
	}

	second, err := s.CreateBug(CreateBug{Title: "Search misses tags", ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if second.Identifier != "B-2" {
		t.Fatalf("second identifier %s", second.Identifier)
	}

	if _, err := s.CreateBug(CreateBug{Title: "No project"}); err == nil {
		t.Fatal("expected missing project to fail")
	}

	proj, err := s.GetProject(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.Bugs) != 2 || proj.Bugs[0].Identifier != "B-1" || proj.Bugs[1].Identifier != "B-2" {
		t.Fatalf("project bugs: %+v", proj.Bugs)
	}

	open := s.ListBugs(p.ID, model.StateOpen)
	if len(open) != 2 {
		t.Fatalf("open: %d", len(open))
	}
	closed := model.StateClosed
	if _, err := s.Update(first.ID, UpdateIssue{State: &closed}); err != nil {
		t.Fatal(err)
	}
	if got := s.ListBugs(p.ID, model.StateOpen); len(got) != 1 || got[0].ID != second.ID {
		t.Fatalf("after close: %+v", got)
	}

	status, err := s.ProjectStatus(&p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if status.Progress.Bugs.Total != 2 || status.Progress.Bugs.Open != 1 || status.Progress.Bugs.Closed != 1 {
		t.Fatalf("status bugs: %+v", status.Progress.Bugs)
	}
	if status.Progress.Tickets.Total != 0 {
		t.Fatalf("bugs should not count as tickets: %+v", status.Progress.Tickets)
	}

	front := s.Frontier(nil)
	found := false
	for _, issue := range front {
		if issue.ID == second.ID {
			found = true
		}
		if issue.ID == first.ID {
			t.Fatal("closed bug on frontier")
		}
	}
	if !found {
		t.Fatalf("open bug should be on frontier: %+v", ids(front))
	}

	if _, err := s.Wipe(); err != nil {
		t.Fatal(err)
	}
	p2, err := s.CreateProject(CreateProject{Title: "Again"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateBug(CreateBug{Title: "First after wipe", ProjectID: p2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if again.Identifier != "B-1" {
		t.Fatalf("after wipe identifier %s", again.Identifier)
	}
}
