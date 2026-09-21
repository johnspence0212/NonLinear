package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/johnspence0212/NonLinear/internal/model"
)

func TestLegacyFixtureDecodes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "legacy-v0.json"))
	if err != nil {
		t.Fatal(err)
	}
	db, extra, err := DecodeDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	if extra != nil {
		t.Fatalf("unexpected extra keys: %v", extra)
	}
	if db.SchemaVersion != 0 {
		t.Fatalf("legacy schemaVersion: %d", db.SchemaVersion)
	}
	if db.NextID != 5 || db.Prefix != "NL" || len(db.Issues) != 4 {
		t.Fatalf("envelope: next=%d prefix=%s issues=%d", db.NextID, db.Prefix, len(db.Issues))
	}
	byID := map[int]model.Issue{}
	for _, issue := range db.Issues {
		byID[issue.ID] = issue
	}
	m1 := byID[1]
	if m1.Title != "TUI map" || m1.State != model.StateOpen || !model.IsMap(m1) {
		t.Fatalf("map 1: %+v", m1)
	}
	if m1.Kind != model.KindDecisionMap || m1.Lifecycle != model.MapLifecycleActive {
		t.Fatalf("map 1 kind/lifecycle: %s %s", m1.Kind, m1.Lifecycle)
	}
	if len(m1.LinkedMaps) != 1 || m1.LinkedMaps[0] != 3 {
		t.Fatalf("map 1 linked: %v", m1.LinkedMaps)
	}
	if len(m1.Comments) != 1 || m1.Comments[0].Body != "kickoff" {
		t.Fatalf("map 1 comments: %+v", m1.Comments)
	}
	t2 := byID[2]
	if t2.ParentID == nil || *t2.ParentID != 1 {
		t.Fatalf("ticket 2 parent: %+v", t2.ParentID)
	}
	if model.AssigneeValue(t2) != "cursor" {
		t.Fatalf("ticket 2 claim: %q", model.AssigneeValue(t2))
	}
	if !model.HasLabel(t2, "ready-for-agent") {
		t.Fatalf("ticket 2 labels: %v", t2.Labels)
	}
	t4 := byID[4]
	if len(t4.BlockedBy) != 1 || t4.BlockedBy[0] != 2 {
		t.Fatalf("ticket 4 blockers: %v", t4.BlockedBy)
	}
	if !containsLabel(db.Labels, "custom-tag") || !containsLabel(db.Labels, "wayfinder:map") {
		t.Fatalf("labels: %v", db.Labels)
	}
}

func TestLegacyMapsDoNotInventProjects(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "legacy-v0.json"))
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := DecodeDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(db.Projects) != 0 {
		t.Fatalf("projects: %+v", db.Projects)
	}
	for _, issue := range db.Issues {
		if issue.ProjectID != nil {
			t.Fatalf("issue %d projectId: %v", issue.ID, issue.ProjectID)
		}
	}
}

func TestOpenLegacyDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile(filepath.Join("testdata", "legacy-v0.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "db.json")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Millisecond)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.ModTime() != before.ModTime() || after.Size() != before.Size() {
		t.Fatalf("Open wrote the file: mtime %v -> %v size %d -> %d", before.ModTime(), after.ModTime(), before.Size(), after.Size())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(src) {
		t.Fatal("Open changed file bytes")
	}
	m, err := s.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if m.ProjectRef != nil {
		t.Fatalf("standalone map should have no project: %+v", m.ProjectRef)
	}
}

func TestUnsupportedSchemaDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.json")
	src := []byte("{\n  \"schemaVersion\": 99,\n  \"nextId\": 1,\n  \"prefix\": \"NL\",\n  \"issues\": []\n}\n")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Open(path)
	if err == nil || err.Error() != "unsupported schema version 99" {
		t.Fatalf("err: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.ModTime() != before.ModTime() {
		t.Fatal("unsupported schema wrote the file")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(src) {
		t.Fatal("unsupported schema changed bytes")
	}
}

func TestUnknownTopLevelKeysRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.json")
	src := []byte("{\n  \"nextId\": 1,\n  \"prefix\": \"NL\",\n  \"issues\": [],\n  \"labels\": [],\n  \"customNote\": \"keep me\"\n}\n")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(CreateIssue{Title: "Map", Labels: []string{"wayfinder:map"}}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	db, extra, err := DecodeDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	if db.SchemaVersion != model.SchemaVersion {
		t.Fatalf("schemaVersion %d", db.SchemaVersion)
	}
	if string(extra["customNote"]) != `"keep me"` {
		t.Fatalf("extra: %v", extra)
	}
}

func containsLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}
