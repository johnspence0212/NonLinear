package mcpserver

import (
	"testing"

	"github.com/johnspence0212/NonLinear/internal/model"
)

func TestMCPDependenciesAuthoritative(t *testing.T) {
	ctx, session := startMCP(t)
	parent := intID(callOK(t, ctx, session, "create_issue", map[string]any{
		"title": "Map", "labels": []string{"wayfinder:map"},
	}))
	a := callOK(t, ctx, session, "create_issue", map[string]any{
		"title": "NL-178 stand-in", "parentId": parent,
	})
	b := callOK(t, ctx, session, "create_issue", map[string]any{
		"title": "NL-179 stand-in", "parentId": parent,
	})
	if blockedBy, _ := b["blockedBy"].([]any); len(blockedBy) != 0 {
		t.Fatalf("create should start unblocked: %v", b)
	}
	if b["frontier"] != true {
		t.Fatalf("new ticket should be frontier: %v", b)
	}

	set := callOK(t, ctx, session, "set_blocked_by", map[string]any{
		"id": intID(b), "issueIds": []any{intID(a)},
	})
	assertBlocked(t, set, intID(a))

	got := callOK(t, ctx, session, "get_issue", map[string]any{"id": intID(b)})
	assertBlocked(t, got, intID(a))
	if _, ok := got["body"]; !ok {
		t.Fatal("get_issue must still return the full object")
	}

	alias := callOK(t, ctx, session, "set_blocked_by", map[string]any{
		"id": intID(b), "blockedBy": []any{intID(a)},
	})
	assertBlocked(t, alias, intID(a))

	created := callOK(t, ctx, session, "create_issue", map[string]any{
		"title": "Depends on A", "parentId": parent, "blockedBy": []any{intID(a)},
	})
	assertBlocked(t, created, intID(a))

	resolved := callOK(t, ctx, session, "resolve_issue", map[string]any{
		"id": intID(a), "answer": "JSON on disk.",
	})
	if resolved["state"] != "closed" {
		t.Fatalf("resolve: %v", resolved)
	}
	after := callOK(t, ctx, session, "get_issue", map[string]any{"id": intID(b)})
	if after["blocked"] != false || after["frontier"] != true || after["openBlockers"] != float64(0) {
		t.Fatalf("after blocker resolved: %v", after)
	}
	front := callOK(t, ctx, session, "list_frontier", map[string]any{"parentId": parent})
	next, _ := front["next"].(map[string]any)
	if intID(next) != intID(b) {
		t.Fatalf("frontier next after resolve: %v", front)
	}
	if _, ok := next["body"]; ok {
		t.Fatalf("list_frontier next should be a summary: %v", next)
	}

	cleared := callOK(t, ctx, session, "set_blocked_by", map[string]any{
		"id": intID(b), "issueIds": []any{},
	})
	if blockedBy, _ := cleared["blockedBy"].([]any); len(blockedBy) != 0 {
		t.Fatalf("cleared: %v", cleared)
	}
	if cleared["blocked"] != false || cleared["frontier"] != true {
		t.Fatalf("cleared derived: %v", cleared)
	}
}

func TestMCPWriteAndListContracts(t *testing.T) {
	ctx, session := startMCP(t)
	created := callOK(t, ctx, session, "create_issue", map[string]any{
		"title": "Ticket", "body": "Do not echo this body back.",
	})
	for _, key := range []string{"body", "comments", "children", "parent", "projectRef"} {
		if _, ok := created[key]; ok {
			t.Fatalf("create_issue leaked %s: %v", key, created)
		}
	}
	if created["created"] != true || created["updated"] != true {
		t.Fatalf("create ack: %v", created)
	}

	updated := callOK(t, ctx, session, "update_issue", map[string]any{
		"id": intID(created), "body": "Still should not echo.",
	})
	if _, ok := updated["body"]; ok {
		t.Fatalf("update_issue leaked body: %v", updated)
	}

	listed := callOK(t, ctx, session, "list_issues", map[string]any{})
	issues, _ := listed["issues"].([]any)
	if len(issues) == 0 {
		t.Fatal("list_issues empty")
	}
	row, _ := issues[0].(map[string]any)
	for _, key := range []string{"body", "comments", "children"} {
		if _, ok := row[key]; ok {
			t.Fatalf("list_issues leaked %s: %v", key, row)
		}
	}

	detail := callOK(t, ctx, session, "get_issue", map[string]any{"id": intID(created)})
	if detail["body"] != "Still should not echo." {
		t.Fatalf("get_issue should keep the body: %v", detail)
	}
}

func TestMCPRejectSpecBodyOnMap(t *testing.T) {
	ctx, session := startMCP(t)
	m := callOK(t, ctx, session, "create_issue", map[string]any{
		"title": "Map", "labels": []string{"wayfinder:map"}, "body": "## Destination\n\nShip.\n",
	})
	specBody := model.SpecSkeleton("Ship.")
	msg := callErr(t, ctx, session, "update_issue", map[string]any{
		"id": intID(m), "body": specBody,
	})
	if msg == "" {
		t.Fatal("expected spec-on-map error")
	}

	spec := callOK(t, ctx, session, "advance_to_spec", map[string]any{"id": intID(m)})
	if spec["kind"] != "spec" || spec["created"] != true {
		t.Fatalf("advance_to_spec: %v", spec)
	}
	if _, ok := spec["body"]; ok {
		t.Fatalf("advance_to_spec should not return the skeleton body: %v", spec)
	}

	kindMsg := callErr(t, ctx, session, "update_issue", map[string]any{
		"id": intID(m), "kind": "spec", "body": specBody,
	})
	if kindMsg == "" {
		t.Fatal("expected kind mismatch")
	}

	filled := callOK(t, ctx, session, "update_issue", map[string]any{
		"id": intID(spec), "kind": "spec", "body": specBody,
	})
	if filled["kind"] != "spec" {
		t.Fatalf("spec update: %v", filled)
	}
	got := callOK(t, ctx, session, "get_issue", map[string]any{"id": intID(spec)})
	if got["body"] != specBody {
		t.Fatal("spec body was not stored")
	}
}

func assertBlocked(t *testing.T, obj map[string]any, blockerID int) {
	t.Helper()
	ids, _ := obj["blockedBy"].([]any)
	if len(ids) != 1 || int(ids[0].(float64)) != blockerID {
		t.Fatalf("blockedBy=%v want [%d] in %v", ids, blockerID, obj)
	}
	if obj["blocked"] != true || obj["frontier"] != false || obj["openBlockers"] != float64(1) {
		t.Fatalf("derived blocked state: %v", obj)
	}
}
