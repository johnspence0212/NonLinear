package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/johnspence0212/NonLinear/internal/model"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateAndAtomicSave(t *testing.T) {
	s := testStore(t)
	issue, err := s.Create(CreateIssue{Title: "Chart the destination", Labels: []string{"wayfinder:map"}})
	if err != nil {
		t.Fatal(err)
	}
	if issue.Identifier != "NL-1" {
		t.Fatalf("identifier %s", issue.Identifier)
	}
	raw, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	var db model.DB
	if err := json.Unmarshal(raw, &db); err != nil {
		t.Fatal(err)
	}
	if db.NextID != 2 || len(db.Issues) != 1 {
		t.Fatalf("saved db: %+v", db)
	}
}

func TestFrontierBlockedClaimedClosed(t *testing.T) {
	s := testStore(t)
	m, err := s.Create(CreateIssue{Title: "Map", Labels: []string{"wayfinder:map"}})
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

	front := s.Frontier(&parent)
	if len(front) != 1 || front[0].ID != a.ID {
		t.Fatalf("expected only A on frontier, got %+v", ids(front))
	}

	got, err := s.Get(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Blocked || got.Frontier {
		t.Fatalf("B should be blocked: %+v", got)
	}

	if _, err := s.Claim(a.ID, "cursor"); err != nil {
		t.Fatal(err)
	}
	front = s.Frontier(&parent)
	if len(front) != 0 {
		t.Fatalf("claimed A should leave frontier empty, got %+v", ids(front))
	}

	if _, err := s.Resolve(a.ID, "cursor", "JSON file with atomic writes."); err != nil {
		t.Fatal(err)
	}
	front = s.Frontier(&parent)
	if len(front) != 1 || front[0].ID != b.ID {
		t.Fatalf("after resolving A, B should be frontier, got %+v", ids(front))
	}

	closed, err := s.Get(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.State != model.StateClosed || closed.Frontier {
		t.Fatalf("A should be closed: %+v", closed)
	}
	if len(closed.Comments) != 1 {
		t.Fatalf("expected resolution comment, got %d", len(closed.Comments))
	}
}

func TestParentCycleRejected(t *testing.T) {
	s := testStore(t)
	a, _ := s.Create(CreateIssue{Title: "A"})
	b, _ := s.Create(CreateIssue{Title: "B", ParentID: &a.ID})
	parent := &b.ID
	if _, err := s.Update(a.ID, UpdateIssue{ParentID: &parent}); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestListFilters(t *testing.T) {
	s := testStore(t)
	_, _ = s.Create(CreateIssue{Title: "Map", Labels: []string{"wayfinder:map"}, Project: "app"})
	unassigned := ""
	list := s.List(ListFilter{Labels: []string{"wayfinder:map"}, Assignee: &unassigned, Project: "app"})
	if len(list) != 1 {
		t.Fatalf("got %d", len(list))
	}
}

func TestDeleteCascadesAndCleansBlockers(t *testing.T) {
	s := testStore(t)
	m, err := s.Create(CreateIssue{Title: "Map", Labels: []string{"wayfinder:map"}})
	if err != nil {
		t.Fatal(err)
	}
	parent := m.ID
	a, err := s.Create(CreateIssue{Title: "A", ParentID: &parent})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(CreateIssue{Title: "B", ParentID: &parent})
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := s.Create(CreateIssue{Title: "Orphan"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBlockedBy(orphan.ID, []int{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	grand := a.ID
	if _, err := s.Create(CreateIssue{Title: "A.1", ParentID: &grand}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Delete(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Deleted) != 4 {
		t.Fatalf("deleted %v", got.Deleted)
	}
	if s.Count() != 1 {
		t.Fatalf("count %d", s.Count())
	}
	left, err := s.Get(orphan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(left.BlockedBy) != 0 {
		t.Fatalf("blockedBy should be empty, got %v", left.BlockedBy)
	}
}

func TestWipeResetsIDs(t *testing.T) {
	s := testStore(t)
	if _, err := s.Create(CreateIssue{Title: "Gone"}); err != nil {
		t.Fatal(err)
	}
	n, err := s.Wipe()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || s.Count() != 0 {
		t.Fatalf("wipe n=%d count=%d", n, s.Count())
	}
	fresh, err := s.Create(CreateIssue{Title: "Again"})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Identifier != "NL-1" {
		t.Fatalf("identifier %s", fresh.Identifier)
	}
}

func TestDeleteMissing(t *testing.T) {
	s := testStore(t)
	if _, err := s.Delete(99); err == nil {
		t.Fatal("expected not found")
	}
}

func ids(views []model.IssueView) []int {
	out := make([]int, len(views))
	for i, v := range views {
		out[i] = v.ID
	}
	return out
}
