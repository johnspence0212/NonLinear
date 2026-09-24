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

func TestUpdateMapLabels(t *testing.T) {
	s := testStore(t)
	m, err := s.Create(CreateIssue{Title: "First session", Labels: []string{"wayfinder:map"}})
	if err != nil {
		t.Fatal(err)
	}
	next := []string{"wayfinder:map", "#focus"}
	got, err := s.Update(m.ID, UpdateIssue{Labels: &next})
	if err != nil {
		t.Fatal(err)
	}
	if !model.HasLabel(got.Issue, "wayfinder:map") || !model.HasLabel(got.Issue, "focus") {
		t.Fatalf("labels: %v", got.Labels)
	}
	found := false
	for _, l := range s.Labels() {
		if l == "focus" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("catalog missing focus")
	}
	bad := []string{"has space"}
	if _, err := s.Update(m.ID, UpdateIssue{Labels: &bad}); err == nil {
		t.Fatal("expected space error")
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
	p, err := s.CreateProject(CreateProject{Title: "Idle Frontier"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Create(CreateIssue{Title: "First session", Labels: []string{"wayfinder:map"}, ProjectID: &p.ID})
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
	if a.ProjectID == nil || b.ProjectID == nil || *a.ProjectID != *b.ProjectID || *a.ProjectID != p.ID {
		t.Fatalf("linkedMapId should join the source project: a=%v b=%v", a.ProjectID, b.ProjectID)
	}
	proj, err := s.GetProject(p.ID)
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

func TestExportProjectIncludesArtifactsAndDropsExternalEdges(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject(CreateProject{Title: "Ship tracker", Destination: "A local tracker."})
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Create(CreateIssue{Title: "Chart the destination", Labels: []string{"wayfinder:map"}, Body: "## Destination\n\nA tracker.\n", ProjectID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateProject(CreateProject{Title: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	outsideMap, err := s.Create(CreateIssue{Title: "Other map", Labels: []string{"wayfinder:map"}, ProjectID: &other.ID})
	if err != nil {
		t.Fatal(err)
	}
	outsideTicket, err := s.Create(CreateIssue{Title: "Outside ticket"})
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
	if _, err := s.SetBlockedBy(b.ID, []int{a.ID, outsideTicket.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLinkedMaps(m.ID, []int{outsideMap.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddComment(a.ID, "cursor", "JSON on disk."); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceToSpec(m.ID); err != nil {
		t.Fatal(err)
	}
	spec, err := s.EnsureSpec(m.ID)
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
	planID := plan.ID
	if _, err := s.Create(CreateIssue{Title: "Write the store", ParentID: &planID}); err != nil {
		t.Fatal(err)
	}

	bundle, err := s.ExportProject(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Kind != model.ProjectBundleKind || bundle.Version != model.ProjectBundleVersion {
		t.Fatalf("header: %+v", bundle)
	}
	if bundle.Project.ID != p.ID || bundle.Project.Title != "Ship tracker" {
		t.Fatalf("project: %+v", bundle.Project)
	}
	if bundle.Filename() != "P-1-ship-tracker.nlproject.json" {
		t.Fatalf("filename %s", bundle.Filename())
	}
	if len(bundle.Issues) != 6 {
		t.Fatalf("issues %d", len(bundle.Issues))
	}
	byID := map[int]model.Issue{}
	for _, issue := range bundle.Issues {
		byID[issue.ID] = issue
		if issue.ProjectID == nil || *issue.ProjectID != p.ID {
			t.Fatalf("projectId %v want %d", issue.ProjectID, p.ID)
		}
	}
	gotB := byID[b.ID]
	if len(gotB.BlockedBy) != 1 || gotB.BlockedBy[0] != a.ID {
		t.Fatalf("external blocker should be dropped: %v", gotB.BlockedBy)
	}
	if len(byID[m.ID].LinkedMaps) != 0 {
		t.Fatalf("external linked map should be dropped: %v", byID[m.ID].LinkedMaps)
	}
	if len(byID[a.ID].Comments) != 1 {
		t.Fatalf("comments: %+v", byID[a.ID].Comments)
	}
	gotSpec := byID[spec.ID]
	if gotSpec.Kind != model.KindSpec || gotSpec.DerivedFromArtifactID == nil || *gotSpec.DerivedFromArtifactID != m.ID {
		t.Fatalf("spec: %+v", gotSpec)
	}
	gotPlan := byID[plan.ID]
	if gotPlan.Kind != model.KindPlan || gotPlan.DerivedFromArtifactID == nil || *gotPlan.DerivedFromArtifactID != spec.ID {
		t.Fatalf("plan: %+v", gotPlan)
	}
	if _, err := s.ExportProject(99); err == nil {
		t.Fatal("export of missing project should fail")
	}
}

func TestImportProjectRemapsIDsAndPreservesGraph(t *testing.T) {
	src := testStore(t)
	p, err := src.CreateProject(CreateProject{Title: "Ship tracker", Destination: "A local tracker."})
	if err != nil {
		t.Fatal(err)
	}
	m, err := src.Create(CreateIssue{Title: "Chart the destination", Labels: []string{"wayfinder:map"}, Body: "## Destination\n\nA tracker.\n", ProjectID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	parent := m.ID
	a, err := src.Create(CreateIssue{Title: "What store?", Labels: []string{"wayfinder:grilling"}, ParentID: &parent})
	if err != nil {
		t.Fatal(err)
	}
	b, err := src.Create(CreateIssue{Title: "How to expose MCP?", Labels: []string{"wayfinder:grilling"}, ParentID: &parent})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.SetBlockedBy(b.ID, []int{a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Resolve(a.ID, "cursor", "JSON file with atomic writes."); err != nil {
		t.Fatal(err)
	}
	if _, err := src.AdvanceToSpec(m.ID); err != nil {
		t.Fatal(err)
	}
	spec, err := src.EnsureSpec(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.ApproveSpec(spec.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := src.CreatePlan(spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	planID := plan.ID
	if _, err := src.Create(CreateIssue{Title: "Write the store", ParentID: &planID}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.SetLinkedMaps(m.ID, []int{}); err != nil {
		t.Fatal(err)
	}
	second, err := src.Create(CreateIssue{Title: "Second session", Labels: []string{"wayfinder:map"}, ProjectID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.SetLinkedMaps(m.ID, []int{second.ID}); err != nil {
		t.Fatal(err)
	}
	bundle, err := src.ExportProject(p.ID)
	if err != nil {
		t.Fatal(err)
	}

	dst := testStore(t)
	existing, err := dst.CreateProject(CreateProject{Title: "Already here"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Create(CreateIssue{Title: "Noise"}); err != nil {
		t.Fatal(err)
	}
	got, err := dst.ImportProject(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project.ID == p.ID || got.Project.ID == existing.ID {
		t.Fatalf("imported project should get a new id, got %d", got.Project.ID)
	}
	if got.Project.Title != "Ship tracker" || got.Project.Destination != "A local tracker." {
		t.Fatalf("project: %+v", got.Project)
	}
	if got.Project.Identifier != "P-2" {
		t.Fatalf("identifier %s", got.Project.Identifier)
	}
	if len(got.Created) != 7 {
		t.Fatalf("created %v", got.Created)
	}
	if len(got.Project.Maps) != 2 || len(got.Project.Specs) != 1 || len(got.Project.Plans) != 1 {
		t.Fatalf("artifacts maps=%d specs=%d plans=%d", len(got.Project.Maps), len(got.Project.Specs), len(got.Project.Plans))
	}

	var chart, secondMap model.IssueView
	for _, item := range got.Project.Maps {
		switch item.Title {
		case "Chart the destination":
			chart = item
		case "Second session":
			secondMap = item
		}
	}
	if chart.ID == 0 || secondMap.ID == 0 {
		t.Fatalf("maps: %+v", got.Project.Maps)
	}
	if chart.ProjectID == nil || *chart.ProjectID != got.Project.ID {
		t.Fatalf("map projectId %v want %d", chart.ProjectID, got.Project.ID)
	}
	if len(chart.Linked) != 1 || chart.Linked[0].ID != secondMap.ID {
		t.Fatalf("linked maps remapped: %+v", chart.Linked)
	}
	kids := dst.List(ListFilter{ParentID: &chart.ID})
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
	specView := got.Project.Specs[0]
	if specView.DerivedFromArtifactID == nil || *specView.DerivedFromArtifactID != chart.ID {
		t.Fatalf("spec derivedFrom: %+v want map %d", specView.DerivedFromArtifactID, chart.ID)
	}
	planView := got.Project.Plans[0]
	if planView.DerivedFromArtifactID == nil || *planView.DerivedFromArtifactID != specView.ID {
		t.Fatalf("plan derivedFrom: %+v want spec %d", planView.DerivedFromArtifactID, specView.ID)
	}
	planKids := dst.List(ListFilter{ParentID: &planView.ID})
	if len(planKids) != 1 || planKids[0].Title != "Write the store" {
		t.Fatalf("plan kids: %+v", planKids)
	}

	again, err := dst.ImportProject(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if again.Project.ID == got.Project.ID {
		t.Fatal("second import should create another project")
	}
	if dst.Count() != 1+7+7 {
		t.Fatalf("count %d", dst.Count())
	}
}

func TestImportProjectEmptyAndRejectsBadBundle(t *testing.T) {
	src := testStore(t)
	p, err := src.CreateProject(CreateProject{Title: "Empty box"})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := src.ExportProject(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Issues) != 0 {
		t.Fatalf("empty export: %+v", bundle)
	}
	dst := testStore(t)
	got, err := dst.ImportProject(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project.Title != "Empty box" || len(got.Created) != 0 {
		t.Fatalf("empty import: %+v", got)
	}
	if _, err := dst.ImportProject(model.ProjectBundle{Kind: "nope", Project: model.Project{Title: "X"}}); err == nil {
		t.Fatal("expected kind error")
	}
	if _, err := dst.ImportProject(model.ProjectBundle{Kind: model.ProjectBundleKind, Version: 99, Project: model.Project{Title: "X"}}); err == nil {
		t.Fatal("expected version error")
	}
	if _, err := dst.ImportProject(model.ProjectBundle{Kind: model.ProjectBundleKind, Version: 1}); err == nil {
		t.Fatal("expected title error")
	}
}

func TestImportProjectDropsMissingRepo(t *testing.T) {
	src := testStore(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := src.CreateProject(CreateProject{Title: "With repo", Repo: root})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := src.ExportProject(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Project.Repo != root {
		t.Fatalf("exported repo %q", bundle.Project.Repo)
	}
	same, err := src.ImportProject(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if same.Project.Repo != root {
		t.Fatalf("same-machine repo %q want %q", same.Project.Repo, root)
	}
	bundle.Project.Repo = filepath.Join(root, "missing-elsewhere")
	dst := testStore(t)
	got, err := dst.ImportProject(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project.Repo != "" {
		t.Fatalf("missing repo should be dropped, got %q", got.Project.Repo)
	}
}

func ids(views []model.IssueView) []int {
	out := make([]int, len(views))
	for i, v := range views {
		out[i] = v.ID
	}
	return out
}
