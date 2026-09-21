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
	if len(got.Blockers) != 1 || got.Blockers[0].ID != a.ID {
		t.Fatalf("B blockers: %+v", got.Blockers)
	}
	blocker, err := s.Get(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocker.Blocks) != 1 || blocker.Blocks[0].ID != b.ID {
		t.Fatalf("A should block B: %+v", blocker.Blocks)
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

func TestGlobalFrontierExcludesMapsAndClosed(t *testing.T) {
	s := testStore(t)
	m, err := s.Create(CreateIssue{Title: "Map", Labels: []string{"wayfinder:map"}})
	if err != nil {
		t.Fatal(err)
	}
	parent := m.ID
	done, err := s.Create(CreateIssue{Title: "Already answered", Labels: []string{"wayfinder:research"}, ParentID: &parent})
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.Create(CreateIssue{Title: "Next question", Labels: []string{"wayfinder:grilling"}, ParentID: &parent})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(done.ID, "cursor", "Done."); err != nil {
		t.Fatal(err)
	}

	front := s.Frontier(nil)
	if len(front) != 1 || front[0].ID != next.ID {
		t.Fatalf("expected only next ticket, got %+v", ids(front))
	}
	got, err := s.Get(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Frontier {
		t.Fatalf("map should not be frontier: %+v", got)
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

func TestUpdateComment(t *testing.T) {
	s := testStore(t)
	issue, err := s.Create(CreateIssue{Title: "Ticket"})
	if err != nil {
		t.Fatal(err)
	}
	added, err := s.AddComment(issue.ID, "me", "first **draft**")
	if err != nil {
		t.Fatal(err)
	}
	if len(added.Comments) != 1 {
		t.Fatalf("comments: %+v", added.Comments)
	}
	cid := added.Comments[0].ID
	got, err := s.UpdateComment(issue.ID, cid, "edited **body**")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Comments) != 1 || got.Comments[0].Body != "edited **body**" {
		t.Fatalf("updated: %+v", got.Comments)
	}
	if got.Comments[0].UpdatedAt == nil || got.Comments[0].UpdatedAt.IsZero() {
		t.Fatal("expected updatedAt")
	}
	if _, err := s.UpdateComment(issue.ID, "missing", "nope"); err == nil {
		t.Fatal("expected missing comment")
	}
	if _, err := s.UpdateComment(issue.ID, cid, "  "); err == nil {
		t.Fatal("expected empty body error")
	}
}

func TestCreateAndAddLabel(t *testing.T) {
	s := testStore(t)
	got, err := s.CreateLabel("#sprint-12")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Created || got.Label != "sprint-12" {
		t.Fatalf("create: %+v", got)
	}
	found := false
	for _, l := range got.Labels {
		if l == "sprint-12" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("catalog missing sprint-12: %v", got.Labels)
	}
	again, err := s.CreateLabel("sprint-12")
	if err != nil {
		t.Fatal(err)
	}
	if again.Created {
		t.Fatal("second create should be idempotent")
	}
	if _, err := s.CreateLabel("has space"); err == nil {
		t.Fatal("expected space error")
	}
	if _, err := s.CreateLabel(""); err == nil {
		t.Fatal("expected empty error")
	}

	issue, err := s.Create(CreateIssue{Title: "Ticket", Labels: []string{"needs-triage"}})
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := s.AddLabel(issue.ID, "sprint-12")
	if err != nil {
		t.Fatal(err)
	}
	if !model.HasLabel(tagged.Issue, "needs-triage") || !model.HasLabel(tagged.Issue, "sprint-12") {
		t.Fatalf("labels replaced instead of appended: %v", tagged.Labels)
	}
	if _, err := s.Wipe(); err != nil {
		t.Fatal(err)
	}
	after := s.Labels()
	for _, l := range after {
		if l == "sprint-12" {
			t.Fatal("wipe should drop catalog labels")
		}
	}
}

func TestLinkedMapsAreSymmetricAndDoNotCascade(t *testing.T) {
	s := testStore(t)
	a, err := s.Create(CreateIssue{Title: "First session", Labels: []string{"wayfinder:map"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(CreateIssue{Title: "Spawned session", LinkedMapID: &a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !model.IsMap(b.Issue) {
		t.Fatal("linkedMapId should make a map")
	}
	if a.ProjectID == nil || b.ProjectID == nil || *a.ProjectID != *b.ProjectID {
		t.Fatalf("linkedMapId should join the source project: a=%v b=%v", a.ProjectID, b.ProjectID)
	}
	proj, err := s.GetProject(*a.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.Maps) != 2 {
		t.Fatalf("project maps: %+v", ids(proj.Maps))
	}
	if len(b.Linked) != 1 || b.Linked[0].ID != a.ID {
		t.Fatalf("new map should link back: %+v", b.Linked)
	}
	gotA, err := s.Get(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotA.Linked) != 1 || gotA.Linked[0].ID != b.ID {
		t.Fatalf("origin should show the new map: %+v", gotA.Linked)
	}

	ticket, err := s.Create(CreateIssue{Title: "Not a map"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLinkedMaps(a.ID, []int{ticket.ID}); err == nil {
		t.Fatal("should not link a ticket")
	}
	if _, err := s.SetLinkedMaps(a.ID, []int{a.ID}); err == nil {
		t.Fatal("should not self-link")
	}

	if _, err := s.Delete(b.ID); err != nil {
		t.Fatal(err)
	}
	gotA, err = s.Get(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotA.Linked) != 0 {
		t.Fatalf("delete should unlink, not delete origin: %+v", gotA.Linked)
	}
	if s.Count() != 2 {
		t.Fatalf("origin and ticket should remain, count=%d", s.Count())
	}
}

func TestExportMapDropsExternalBlockersAndIncludesDescendants(t *testing.T) {
	s := testStore(t)
	m, err := s.Create(CreateIssue{Title: "Chart the destination", Labels: []string{"wayfinder:map"}})
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
	outside, err := s.Create(CreateIssue{Title: "Other map ticket"})
	if err != nil {
		t.Fatal(err)
	}
	grand := a.ID
	if _, err := s.Create(CreateIssue{Title: "Nested", ParentID: &grand}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetBlockedBy(b.ID, []int{a.ID, outside.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddComment(a.ID, "cursor", "JSON on disk."); err != nil {
		t.Fatal(err)
	}

	bundle, err := s.ExportMap(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Kind != model.MapBundleKind || bundle.Version != model.MapBundleVersion {
		t.Fatalf("header: %+v", bundle)
	}
	if bundle.RootID != m.ID || len(bundle.Issues) != 4 {
		t.Fatalf("bundle: root=%d n=%d", bundle.RootID, len(bundle.Issues))
	}
	byID := map[int]model.Issue{}
	for _, issue := range bundle.Issues {
		byID[issue.ID] = issue
	}
	if byID[m.ID].ParentID != nil {
		t.Fatal("exported map should have no parent")
	}
	gotB := byID[b.ID]
	if len(gotB.BlockedBy) != 1 || gotB.BlockedBy[0] != a.ID {
		t.Fatalf("external blocker should be dropped: %v", gotB.BlockedBy)
	}
	if len(byID[a.ID].Comments) != 1 {
		t.Fatalf("comments: %+v", byID[a.ID].Comments)
	}
	if _, err := s.ExportMap(a.ID); err == nil {
		t.Fatal("export of a ticket should fail")
	}
	if _, err := s.ExportMap(99); err == nil {
		t.Fatal("export of missing id should fail")
	}
}

func TestImportMapRemapsIDsAndPreservesGraph(t *testing.T) {
	src := testStore(t)
	m, _ := src.Create(CreateIssue{Title: "Chart the destination", Labels: []string{"wayfinder:map"}, Body: "## Destination\n\nA tracker.\n"})
	parent := m.ID
	a, _ := src.Create(CreateIssue{Title: "What store?", Labels: []string{"wayfinder:grilling"}, ParentID: &parent})
	b, _ := src.Create(CreateIssue{Title: "How to expose MCP?", Labels: []string{"wayfinder:grilling"}, ParentID: &parent})
	if _, err := src.SetBlockedBy(b.ID, []int{a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Resolve(a.ID, "cursor", "JSON file with atomic writes."); err != nil {
		t.Fatal(err)
	}
	bundle, err := src.ExportMap(m.ID)
	if err != nil {
		t.Fatal(err)
	}

	dst := testStore(t)
	existing, _ := dst.Create(CreateIssue{Title: "Already here"})
	got, err := dst.ImportMap(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if got.Map.ID == m.ID || got.Map.ID == existing.ID {
		t.Fatalf("imported map should get a new id, got %d", got.Map.ID)
	}
	if got.Map.Title != "Chart the destination" || !model.IsMap(got.Map.Issue) {
		t.Fatalf("map: %+v", got.Map)
	}
	if got.Map.ParentID != nil {
		t.Fatal("imported map should be top-level")
	}
	if len(got.Created) != 3 {
		t.Fatalf("created %v", got.Created)
	}
	if got.Map.Identifier != "NL-2" {
		t.Fatalf("identifier %s", got.Map.Identifier)
	}
	if len(got.Map.Children) != 2 {
		t.Fatalf("children: %+v", got.Map.Children)
	}

	kids := dst.List(ListFilter{ParentID: &got.Map.ID})
	var storeQ, mcpQ model.IssueView
	for _, k := range kids {
		switch k.Title {
		case "What store?":
			storeQ = k
		case "How to expose MCP?":
			mcpQ = k
		}
	}
	if storeQ.ID == 0 || mcpQ.ID == 0 {
		t.Fatalf("kids: %+v", kids)
	}
	if storeQ.State != model.StateClosed || len(storeQ.Comments) != 1 {
		t.Fatalf("resolved ticket: %+v", storeQ)
	}
	if len(mcpQ.BlockedBy) != 1 || mcpQ.BlockedBy[0] != storeQ.ID {
		t.Fatalf("blockedBy remapped: %v want %d", mcpQ.BlockedBy, storeQ.ID)
	}
	front := dst.Frontier(&got.Map.ID)
	if len(front) != 1 || front[0].ID != mcpQ.ID {
		t.Fatalf("frontier after import: %+v", ids(front))
	}

	again, err := dst.ImportMap(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if again.Map.ID == got.Map.ID {
		t.Fatal("second import should create another map")
	}
	if dst.Count() != 1+3+3 {
		t.Fatalf("count %d", dst.Count())
	}
}

func TestImportMapRejectsBadBundle(t *testing.T) {
	s := testStore(t)
	if _, err := s.ImportMap(model.MapBundle{Kind: "nope", Version: 1, Issues: []model.Issue{{ID: 1, Title: "X"}}}); err == nil {
		t.Fatal("expected kind error")
	}
	if _, err := s.ImportMap(model.MapBundle{Kind: model.MapBundleKind, Version: 99, RootID: 1, Issues: []model.Issue{{ID: 1, Title: "X"}}}); err == nil {
		t.Fatal("expected version error")
	}
	if _, err := s.ImportMap(model.MapBundle{Kind: model.MapBundleKind, Version: 1}); err == nil {
		t.Fatal("expected empty error")
	}
}

func ids(views []model.IssueView) []int {
	out := make([]int, len(views))
	for i, v := range views {
		out[i] = v.ID
	}
	return out
}
