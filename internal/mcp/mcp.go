package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/johnspence0212/NonLinear/internal/store"
	"github.com/johnspence0212/NonLinear/internal/version"
)

func New(st *store.Store) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "nonlinear", Version: version.Version}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_issues",
		Description: "List NonLinear issues. Filter by state (open/closed), labels (AND), parentId (Wayfinder children of a map), assignee (use \"unassigned\" for unclaimed), project, query, or frontier=true for takeable tickets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, any, error) {
		filter := store.ListFilter{
			State:        in.State,
			Labels:       in.Labels,
			ParentID:     in.ParentID,
			Project:      in.Project,
			Query:        in.Query,
			FrontierOnly: in.Frontier,
		}
		if in.Assignee != "" {
			a := in.Assignee
			filter.Assignee = &a
		}
		return textResult(st.List(filter))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_issue",
		Description: "Fetch one issue by id (the tracker's identity). Returns body, comments, children, blockers, and frontier/blocked flags. Use this to zoom into a Wayfinder ticket.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.Get(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_issue",
		Description: "Create an issue. Wayfinder map: labels=[\"wayfinder:map\"]. Child ticket: set parentId to the map id and labels=[\"wayfinder:research|prototype|grilling|task\"]. Wire blocked-by in a second pass with set_blocked_by after ids exist.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in createInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.Create(store.CreateIssue{
			Title:    in.Title,
			Body:     in.Body,
			Labels:   in.Labels,
			ParentID: in.ParentID,
			Project:  in.Project,
			Assignee: optString(in.Assignee),
		})
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_issue",
		Description: "Update an issue. Set state to closed to close. Set assignee to claim; empty string or \"unassigned\" to unclaim. Set parentId to attach a child to a map. Use this to append a line to a Wayfinder map body (Decisions so far).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in updateInput) (*mcp.CallToolResult, any, error) {
		up := store.UpdateIssue{
			Title:   optString(in.Title),
			Body:    in.Body,
			Project: optString(in.Project),
		}
		if in.Labels != nil {
			up.Labels = &in.Labels
		}
		if in.State != "" {
			up.State = &in.State
		}
		if in.Assignee != nil {
			up.Assignee = in.Assignee
		}
		if in.ClearParent {
			var none *int
			up.ParentID = &none
		} else if in.ParentID != nil {
			p := in.ParentID
			up.ParentID = &p
		}
		issue, err := st.Update(in.ID, up)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_comment",
		Description: "Add a comment to an issue. Wayfinder resolve: post the answer as a resolution comment, then close (or call resolve_issue).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in commentInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.AddComment(in.ID, in.Author, in.Body)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_blocked_by",
		Description: "Replace native blocked-by edges for an issue. Wayfinder second pass: after creating child tickets, set each ticket's blockers by issue id. A ticket is unblocked when every blocker is closed. The frontier is open + unblocked + unassigned children.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in blockedInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.SetBlockedBy(in.ID, in.IssueIDs)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_frontier",
		Description: "List takeable Wayfinder tickets: open, unassigned, every blocker closed. Pass parentId of the map to scope to that map's children. First result in id order is the next ticket.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in frontierInput) (*mcp.CallToolResult, any, error) {
		return textResult(st.Frontier(in.ParentID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "claim_issue",
		Description: "Claim a ticket by assigning it. Wayfinder: this is the session's first write before any work. Default assignee is \"cursor\".",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in claimInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.Claim(in.ID, in.Assignee)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "resolve_issue",
		Description: "Post a resolution comment and close the issue. Does not edit the map; after this, update_issue the map to append a Decisions-so-far gist that links the closed ticket by name.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in resolveInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.Resolve(in.ID, in.Author, in.Answer)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_issue",
		Description: "Delete an issue and every descendant (map + all child tickets). Remaining issues lose blocked-by edges that pointed at the deleted ids.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		result, err := st.Delete(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(result)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "wipe_db",
		Description: "Erase every issue and reset ids so the next create is NL-1. Requires confirm=true. Irreversible.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in wipeInput) (*mcp.CallToolResult, any, error) {
		if !in.Confirm {
			return errResult(fmt.Errorf("%w: confirm must be true", store.ErrInvalid))
		}
		n, err := st.Wipe()
		if err != nil {
			return errResult(err)
		}
		return textResult(map[string]any{"ok": true, "deleted": n, "version": version.Version})
	})

	return server
}

type listInput struct {
	State    string   `json:"state,omitempty" jsonschema:"open or closed"`
	Labels   []string `json:"labels,omitempty" jsonschema:"labels that must all be present, e.g. wayfinder:map"`
	ParentID *int     `json:"parentId,omitempty" jsonschema:"only children of this issue id"`
	Assignee string   `json:"assignee,omitempty" jsonschema:"assignee name, or unassigned"`
	Project  string   `json:"project,omitempty"`
	Query    string   `json:"query,omitempty" jsonschema:"substring search on identifier, title, body"`
	Frontier bool     `json:"frontier,omitempty" jsonschema:"if true, only takeable tickets"`
}

type idInput struct {
	ID int `json:"id" jsonschema:"issue id"`
}

type createInput struct {
	Title    string   `json:"title" jsonschema:"issue title; refer to tickets by this name"`
	Body     string   `json:"body,omitempty" jsonschema:"markdown body"`
	Labels   []string `json:"labels,omitempty"`
	ParentID *int     `json:"parentId,omitempty" jsonschema:"parent map id for child tickets"`
	Project  string   `json:"project,omitempty"`
	Assignee string   `json:"assignee,omitempty"`
}

type updateInput struct {
	ID          int      `json:"id" jsonschema:"issue id"`
	Title       string   `json:"title,omitempty"`
	Body        *string  `json:"body,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	State       string   `json:"state,omitempty" jsonschema:"open or closed"`
	Assignee    *string  `json:"assignee,omitempty" jsonschema:"set to claim; empty or unassigned to unclaim"`
	ParentID    *int     `json:"parentId,omitempty"`
	ClearParent bool     `json:"clearParent,omitempty"`
	Project     string   `json:"project,omitempty"`
}

type commentInput struct {
	ID     int    `json:"id" jsonschema:"issue id"`
	Body   string `json:"body" jsonschema:"comment markdown"`
	Author string `json:"author,omitempty"`
}

type blockedInput struct {
	ID       int   `json:"id" jsonschema:"issue that is blocked"`
	IssueIDs []int `json:"issueIds" jsonschema:"ids of issues that block this one"`
}

type frontierInput struct {
	ParentID *int `json:"parentId,omitempty" jsonschema:"map issue id"`
}

type claimInput struct {
	ID       int    `json:"id" jsonschema:"issue id"`
	Assignee string `json:"assignee,omitempty" jsonschema:"defaults to cursor"`
}

type resolveInput struct {
	ID     int    `json:"id" jsonschema:"issue id"`
	Answer string `json:"answer" jsonschema:"resolution comment body"`
	Author string `json:"author,omitempty"`
}

type wipeInput struct {
	Confirm bool `json:"confirm" jsonschema:"must be true to wipe"`
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func textResult(v any) (*mcp.CallToolResult, any, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
	}, v, nil
}

func errResult(err error) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
		IsError: true,
	}, nil, nil
}
