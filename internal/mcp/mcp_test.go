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
