package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPResponseSizes(t *testing.T) {
	ctx, session := startMCP(t)
	sizes := measureMCPResponses(t, ctx, session)
	for _, row := range sizes {
		t.Logf("%-22s text=%d structured=%d", row.Name, row.Text, row.Structured)
	}
}

type sizeRow struct {
	Name       string
	Text       int
	Structured int
}

func measureMCPResponses(t *testing.T, ctx context.Context, session *mcp.ClientSession) []sizeRow {
	t.Helper()
	specBody := strings.Repeat("Specification paragraph. ", 80)
	mapBody := "## Destination\n\nShip NonLinear.\n\n## Notes\n\n" + strings.Repeat("Map note. ", 40)
	proj := callOK(t, ctx, session, "create_project", map[string]any{
		"title": "Idle Frontier", "destination": "A local tracker for agents.",
	})
	pid := intID(proj)
	createdMap := callRaw(t, ctx, session, "create_issue", map[string]any{
		"title": "Chart the destination", "labels": []string{"wayfinder:map"},
		"body": mapBody, "projectId": pid,
	})
	mapID := intID(toolJSON(t, createdMap))
	a := callOK(t, ctx, session, "create_issue", map[string]any{
		"title": "What store?", "labels": []string{"wayfinder:grilling"},
		"parentId": mapID, "body": "Question about persistence.",
	})
	b := callOK(t, ctx, session, "create_issue", map[string]any{
		"title": "How to expose MCP?", "labels": []string{"wayfinder:grilling"},
		"parentId": mapID, "body": "Question about the protocol.",
	})
	commented := callRaw(t, ctx, session, "add_comment", map[string]any{
		"id": intID(a), "body": "Research note: JSON on disk with atomic rename.",
	})
	blocked := callRaw(t, ctx, session, "set_blocked_by", map[string]any{
		"id": intID(b), "issueIds": []any{intID(a)},
	})
	updated := callRaw(t, ctx, session, "update_issue", map[string]any{
		"id": mapID, "body": mapBody + "\n## Decisions so far\n\nWhat store? JSON file.\n",
	})
	spec := callOK(t, ctx, session, "advance_to_spec", map[string]any{"id": mapID})
	filled := callRaw(t, ctx, session, "update_issue", map[string]any{
		"id": intID(spec), "body": modelSpecBody(specBody),
	})
	listed := callRaw(t, ctx, session, "list_issues", map[string]any{})
	detail := callRaw(t, ctx, session, "get_issue", map[string]any{"id": mapID})
	front := callRaw(t, ctx, session, "list_frontier", map[string]any{"parentId": mapID})
	project := callRaw(t, ctx, session, "get_project", map[string]any{"id": pid})
	projects := callRaw(t, ctx, session, "list_projects", map[string]any{})
	status := callRaw(t, ctx, session, "get_project_status", map[string]any{"project": "P-1"})
	return []sizeRow{
		{"create_issue(map)", textLen(createdMap), structLen(createdMap)},
		{"update_issue(map)", textLen(updated), structLen(updated)},
		{"add_comment", textLen(commented), structLen(commented)},
		{"set_blocked_by", textLen(blocked), structLen(blocked)},
		{"update_issue(spec)", textLen(filled), structLen(filled)},
		{"list_issues", textLen(listed), structLen(listed)},
		{"get_issue(map)", textLen(detail), structLen(detail)},
		{"list_frontier", textLen(front), structLen(front)},
		{"get_project", textLen(project), structLen(project)},
		{"list_projects", textLen(projects), structLen(projects)},
		{"get_project_status", textLen(status), structLen(status)},
	}
}

func modelSpecBody(extra string) string {
	return "## Destination\n\nShip NonLinear.\n\n## Problem Statement\n\nAgents drown in tracker payloads.\n\n## Solution\n\nCompact MCP writes.\n\n## User Stories\n\n- Agent creates a ticket and sees only confirmation.\n\n## Implementation Decisions\n\nJSON storage stays.\n\n## Testing Decisions\n\nRegression tests for blockers.\n\n## Out of Scope\n\nUI rewrite.\n\n## Further Notes\n\n" + extra
}

func textLen(res *mcp.CallToolResult) int {
	n := 0
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			n += len(tc.Text)
		}
	}
	return n
}

func structLen(res *mcp.CallToolResult) int {
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return 0
	}
	return len(raw)
}
