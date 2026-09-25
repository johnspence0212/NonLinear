package store

import (
	"fmt"
	"testing"

	"github.com/johnspence0212/NonLinear/internal/model"
)

func TestHomeActivityAndPagingLists(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject(CreateProject{Title: "Tracker", Destination: "Ship it."})
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Create(CreateIssue{Title: "Chart the destination", Labels: []string{"wayfinder:map"}, ProjectID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	parent := m.ID
	tickets := make([]model.IssueView, 0, 7)
	for i := 1; i <= 7; i++ {
		ticket, err := s.Create(CreateIssue{Title: fmt.Sprintf("Ticket %d", i), ParentID: &parent})
		if err != nil {
			t.Fatal(err)
		}
		tickets = append(tickets, ticket)
	}
	if _, err := s.Claim(tickets[5].ID, "cursor"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(tickets[6].ID, "cursor"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBlockedBy(tickets[0].ID, []int{tickets[2].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(tickets[1].ID, "cursor", "JSON on disk."); err != nil {
		t.Fatal(err)
	}

	home := s.Home()
	if home.Focus == nil || home.Focus.ID != p.ID {
		t.Fatalf("focus: %+v", home.Focus)
	}
	if len(home.Claimed) != 2 {
		t.Fatalf("claimed: %d %+v", len(home.Claimed), home.Claimed)
	}
	if len(home.Frontier) != 3 {
		t.Fatalf("frontier: %d", len(home.Frontier))
	}
	if len(home.Blocked) != 1 || home.Blocked[0].ID != tickets[0].ID {
		t.Fatalf("blocked: %+v", home.Blocked)
	}
	if len(home.Events) < 10 {
		t.Fatalf("events: %d", len(home.Events))
	}
	if home.Events[0].Kind != model.EventResolved || home.Events[0].Gist != "JSON on disk." {
		t.Fatalf("latest event: %+v", home.Events[0])
	}
	if home.Today.Resolved != 1 || home.Today.Created < 1 {
		t.Fatalf("today: %+v", home.Today)
	}
}
