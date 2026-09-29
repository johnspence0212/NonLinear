package model

import "testing"

func TestLooksLikeSpecBody(t *testing.T) {
	if !LooksLikeSpecBody(SpecSkeleton("Ship it.")) {
		t.Fatal("spec skeleton should look like a spec")
	}
	mapBody := "## Destination\n\nShip.\n\n## Notes\n\nThinking.\n\n## Decisions so far\n\nNone yet.\n"
	if LooksLikeSpecBody(mapBody) {
		t.Fatal("decision map body should not look like a spec")
	}
	if LooksLikeSpecBody("## Problem Statement\n\nOnly one heading.\n") {
		t.Fatal("a single spec heading is not enough")
	}
}

func TestKindMatches(t *testing.T) {
	m := Issue{Kind: KindDecisionMap, Labels: []string{"wayfinder:map"}}
	s := Issue{Kind: KindSpec}
	p := Issue{Kind: KindPlan}
	ticket := Issue{Title: "Work"}
	if !KindMatches(m, "map") || !KindMatches(m, KindDecisionMap) || KindMatches(m, "spec") {
		t.Fatalf("map: %s", ArtifactKind(m))
	}
	if !KindMatches(s, "spec") || KindMatches(s, "map") {
		t.Fatalf("spec: %s", ArtifactKind(s))
	}
	if !KindMatches(p, "tickets") || !KindMatches(p, KindPlan) {
		t.Fatalf("plan: %s", ArtifactKind(p))
	}
	if !KindMatches(ticket, "ticket") || KindMatches(ticket, "spec") {
		t.Fatalf("ticket: %s", ArtifactKind(ticket))
	}
	if !KindMatches(m, "") {
		t.Fatal("empty kind is a no-op")
	}
}
