package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/johnspence0212/NonLinear/internal/store"
)

func TestMCPCreateAndFrontier(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return New(st)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	created, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_issue",
		Arguments: map[string]any{
			"title":  "Map",
			"labels": []string{"wayfinder:map"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.IsError {
		t.Fatalf("%v", created.Content)
	}
	mapIssue := toolJSON(t, created)
	parentID := int(mapIssue["id"].(float64))

	done, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_issue",
		Arguments: map[string]any{
			"title":    "Already answered",
			"labels":   []string{"wayfinder:research"},
			"parentId": parentID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if done.IsError {
		t.Fatalf("%v", done.Content)
	}
	doneIssue := toolJSON(t, done)

	next, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_issue",
		Arguments: map[string]any{
			"title":    "Next question",
			"labels":   []string{"wayfinder:grilling"},
			"parentId": parentID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.IsError {
		t.Fatalf("%v", next.Content)
	}
	nextIssue := toolJSON(t, next)

	if resolved, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "resolve_issue",
		Arguments: map[string]any{
			"id":     int(doneIssue["id"].(float64)),
			"answer": "Done.",
		},
	}); err != nil {
		t.Fatal(err)
	} else if resolved.IsError {
		t.Fatalf("%v", resolved.Content)
	}

	front, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_frontier",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if front.IsError {
		t.Fatalf("%v", front.Content)
	}
	payload := toolJSON(t, front)
	if _, ok := payload["issues"].([]any); !ok {
		t.Fatalf("list_frontier must return an issues object, got %v", payload)
	}
	gotNext, ok := payload["next"].(map[string]any)
	if !ok {
		t.Fatalf("expected next ticket, got %v", payload["next"])
	}
	if gotNext["id"] != nextIssue["id"] {
		t.Fatalf("next should skip closed and maps, got %v", gotNext)
	}

	listed, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_issues",
		Arguments: map[string]any{"frontier": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if listed.IsError {
		t.Fatalf("%v", listed.Content)
	}
	listPayload := toolJSON(t, listed)
	if _, ok := listPayload["issues"].([]any); !ok {
		t.Fatalf("list_issues must return an issues object, got %v", listPayload)
	}

	exported, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "export_map",
		Arguments: map[string]any{"id": parentID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if exported.IsError {
		t.Fatalf("%v", exported.Content)
	}
	bundle := toolJSON(t, exported)
	if bundle["kind"] != "nonlinear.map" {
		t.Fatalf("export: %v", bundle)
	}

	imported, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "import_map",
		Arguments: bundle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if imported.IsError {
		t.Fatalf("%v", imported.Content)
	}
	importedPayload := toolJSON(t, imported)
	gotMap, ok := importedPayload["map"].(map[string]any)
	if !ok || gotMap["id"] == mapIssue["id"] {
		t.Fatalf("import: %v", importedPayload)
	}

	wiped, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wipe_db",
		Arguments: map[string]any{"confirm": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if wiped.IsError {
		t.Fatalf("%v", wiped.Content)
	}
}

func TestMCPLabels(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return New(st)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	created, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_label",
		Arguments: map[string]any{"label": "#focus"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.IsError {
		t.Fatalf("%v", created.Content)
	}
	payload := toolJSON(t, created)
	if payload["label"] != "focus" || payload["created"] != true {
		t.Fatalf("create_label: %v", payload)
	}

	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_labels", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if listed.IsError {
		t.Fatalf("%v", listed.Content)
	}
	listPayload := toolJSON(t, listed)
	found := false
	for _, v := range listPayload["labels"].([]any) {
		if v == "focus" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("list_labels: %v", listPayload)
	}

	issue, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_issue",
		Arguments: map[string]any{"title": "Ticket"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.IsError {
		t.Fatalf("%v", issue.Content)
	}
	id := int(toolJSON(t, issue)["id"].(float64))
	tagged, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "add_label",
		Arguments: map[string]any{"id": id, "label": "focus"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tagged.IsError {
		t.Fatalf("%v", tagged.Content)
	}
	labels, _ := toolJSON(t, tagged)["labels"].([]any)
	if len(labels) != 1 || labels[0] != "focus" {
		t.Fatalf("add_label: %v", toolJSON(t, tagged))
	}
}

func TestMCPLinkedMaps(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return New(st)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	origin, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_issue",
		Arguments: map[string]any{
			"title":  "Origin",
			"labels": []string{"wayfinder:map"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if origin.IsError {
		t.Fatalf("%v", origin.Content)
	}
	originID := int(toolJSON(t, origin)["id"].(float64))

	spawned, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_issue",
		Arguments: map[string]any{
			"title":       "Spawned session",
			"linkedMapId": originID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if spawned.IsError {
		t.Fatalf("%v", spawned.Content)
	}
	spawnedJSON := toolJSON(t, spawned)
	linked, _ := spawnedJSON["linked"].([]any)
	if len(linked) != 1 {
		t.Fatalf("linked: %v", spawnedJSON)
	}

	cleared, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "set_linked_maps",
		Arguments: map[string]any{
			"id":     originID,
			"mapIds": []any{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.IsError {
		t.Fatalf("%v", cleared.Content)
	}
	if got, _ := toolJSON(t, cleared)["linked"].([]any); len(got) != 0 {
		t.Fatalf("unlink: %v", toolJSON(t, cleared))
	}
}

func TestMCPProjectLifecycle(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return New(st)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	created, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_issue",
		Arguments: map[string]any{
			"title":  "Map",
			"labels": []string{"wayfinder:map"},
			"body":   "## Destination\n\nShip.\n",
		},
	})
	if err != nil || created.IsError {
		t.Fatalf("%v %v", err, created)
	}
	mapID := int(toolJSON(t, created)["id"].(float64))

	ready, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "ready_for_spec",
		Arguments: map[string]any{"id": mapID},
	})
	if err != nil || ready.IsError {
		t.Fatalf("ready: %v %v", err, ready)
	}
	spec, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_spec",
		Arguments: map[string]any{"id": mapID},
	})
	if err != nil || spec.IsError {
		t.Fatalf("spec: %v %v", err, spec)
	}
	specID := int(toolJSON(t, spec)["id"].(float64))
	approved, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "approve_spec",
		Arguments: map[string]any{"id": specID},
	})
	if err != nil || approved.IsError {
		t.Fatalf("approve: %v %v", err, approved)
	}
	plan, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_plan",
		Arguments: map[string]any{"id": specID},
	})
	if err != nil || plan.IsError {
		t.Fatalf("plan: %v %v", err, plan)
	}
	if toolJSON(t, plan)["kind"] != "plan" {
		t.Fatalf("plan: %v", toolJSON(t, plan))
	}
	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_projects", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("list: %v %v", err, listed)
	}
}

func TestMCPMoveAndDeleteProject(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return New(st)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	created, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_issue",
		Arguments: map[string]any{"title": "Solo loop", "labels": []string{"wayfinder:map"}},
	})
	if err != nil || created.IsError {
		t.Fatalf("map: %v %v", err, created)
	}
	srcID := int(toolJSON(t, created)["projectId"].(float64))
	dest, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_project",
		Arguments: map[string]any{"title": "Idle Frontier"},
	})
	if err != nil || dest.IsError {
		t.Fatalf("project: %v %v", err, dest)
	}
	destID := int(toolJSON(t, dest)["id"].(float64))
	moved, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "move_to_project",
		Arguments: map[string]any{"fromProjectId": srcID, "projectId": destID},
	})
	if err != nil || moved.IsError {
		t.Fatalf("move: %v %v", err, moved)
	}
	if n, _ := toolJSON(t, moved)["moved"].([]any); len(n) != 1 {
		t.Fatalf("moved: %v", toolJSON(t, moved))
	}
	gone, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "delete_project",
		Arguments: map[string]any{"id": srcID},
	})
	if err != nil || gone.IsError {
		t.Fatalf("delete: %v %v", err, gone)
	}
}

func toolJSON(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("structured content is not an object: %s", raw)
	}
	return out
}
