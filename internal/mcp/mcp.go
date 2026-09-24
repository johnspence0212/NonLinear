package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/johnspence0212/NonLinear/internal/model"
	"github.com/johnspence0212/NonLinear/internal/store"
	"github.com/johnspence0212/NonLinear/internal/version"
)

func New(st *store.Store) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "nonlinear", Version: version.Version}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_labels",
		Description: "List every label/tag this tracker knows: seed labels (wayfinder:*, triage), catalog labels from create_label, and labels already on issues. Returns {labels:[...]}.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in emptyInput) (*mcp.CallToolResult, any, error) {
		return textResult(map[string]any{"labels": st.Labels()})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_label",
		Description: "Create a new label/tag in the tracker catalog so it appears in the UI even before any issue uses it. Strips a leading #. Idempotent: creating an existing label succeeds and sets created=false. To put a label on an issue, use add_label.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in labelInput) (*mcp.CallToolResult, any, error) {
		result, err := st.CreateLabel(in.Label)
		if err != nil {
			return errResult(err)
		}
		return textResult(result)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_label",
		Description: "Add a label/tag to an issue without replacing existing labels. Also records it in the catalog. Use this to tag a ticket; use create_label if you only want the tag to exist.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in addLabelInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.AddLabel(in.ID, in.Label)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_issues",
		Description: "List NonLinear issues. Returns {issues:[...]}. Filter by state (open/closed), labels (AND), parentId (children of a map or plan), assignee (use \"unassigned\" for unclaimed), project, query, or frontier=true for frontier tickets (open, unblocked, unclaimed). Maps, specs, and plans are never on the frontier. Closed issues are never on the frontier.",
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
		return textResult(map[string]any{"issues": st.List(filter)})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_issue",
		Description: "Fetch one issue by id (the tracker's identity). Returns body, comments, children, blockers (what this waits on), blocks (what waits on this), optional linked map edges, projectRef, and frontier/blocked flags. Sibling maps live on the Project, not via linked edges. Use this to zoom into a Wayfinder ticket.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.Get(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_issue",
		Description: "Create an issue. Wayfinder map: labels=[\"wayfinder:map\"]. Put a map on an existing Project with projectId. Child ticket: set parentId to the map id and labels=[\"wayfinder:research|prototype|grilling|task\"]. Implementation ticket: set parentId to the plan id so it shows in the plan tickets section (a text reference to the plan is not enough). linkedMapId still creates a map and an optional bidirectional edge, and attaches it to the source map's Project — prefer projectId. Wire blocked-by in a second pass with set_blocked_by after ids exist.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in createInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.Create(store.CreateIssue{
			Title:       in.Title,
			Body:        in.Body,
			Labels:      in.Labels,
			ParentID:    in.ParentID,
			LinkedMapID: in.LinkedMapID,
			Project:     in.Project,
			ProjectID:   in.ProjectID,
			Assignee:    optString(in.Assignee),
		})
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_issue",
		Description: "Update an issue. Set state to closed to close. Set assignee to claim; empty string or \"unassigned\" to unclaim. Set parentId to attach a child to a map or plan (implementation tickets belong to the plan id). Use this to append a line to a Wayfinder map body (Decisions so far).",
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
		Name:        "update_comment",
		Description: "Edit an existing comment by comment id. Body is markdown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in updateCommentInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.UpdateComment(in.ID, in.CommentID, in.Body)
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
		Name:        "set_linked_maps",
		Description: "Replace optional bidirectional linked-map edges. Both sides must be maps. Linking does not nest, group, or cascade delete — sibling maps belong to a Project. The map UI does not show these edges. Pass mapIds=[] to unlink. To add another map to a Project, prefer create_issue with projectId.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in linkedMapsInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.SetLinkedMaps(in.ID, in.MapIDs)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_frontier",
		Description: "List frontier Wayfinder tickets: open, unassigned, every blocker closed, not a map/spec/plan. Pass parentId of the map or plan to scope to that parent's children. Returns {next, issues}. Use next as the next ticket — do not pick from get_issue children (those include closed tickets).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in frontierInput) (*mcp.CallToolResult, any, error) {
		return textResult(frontierPayload(st.Frontier(in.ParentID)))
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
		Name:        "export_map",
		Description: "Export one Wayfinder map and all of its child tickets as a portable JSON bundle (kind nonlinear.map). Includes comments and in-map blocked-by edges. Use this file with import_map on another tracker.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		bundle, err := st.ExportMap(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(bundle)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "import_map",
		Description: "Import a single-map JSON bundle from export_map. Allocates new ids, remaps parent and blocked-by edges, and returns the new map. Does not overwrite existing issues; importing twice creates two maps.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in importMapInput) (*mcp.CallToolResult, any, error) {
		result, err := st.ImportMap(in.MapBundle)
		if err != nil {
			return errResult(err)
		}
		return textResult(result)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "export_project",
		Description: "Export one Project and everything on it (Decision Maps, Specs, Tickets lists, child tickets, comments, in-project blocked-by / linked-map / derived-from edges) as a portable JSON bundle (kind nonlinear.project). Use this file with import_project on another tracker.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in projectIDInput) (*mcp.CallToolResult, any, error) {
		bundle, err := st.ExportProject(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(bundle)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "import_project",
		Description: "Import a Project JSON bundle from export_project. Creates a new Project, allocates new issue ids, remaps parent / blocked-by / linked-map / derived-from edges, and returns the new Project. A repo path that is not a directory on this machine is dropped. Does not overwrite existing projects; importing twice creates two projects.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in importProjectInput) (*mcp.CallToolResult, any, error) {
		result, err := st.ImportProject(in.ProjectBundle)
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_projects",
		Description: "List Projects (parent of Decision Map → Spec → Plan). Returns {projects:[...]} with derived stage. A Project identifier looks like P-6 and can coexist with NL-6.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in emptyInput) (*mcp.CallToolResult, any, error) {
		return textResult(map[string]any{"projects": st.ListProjects()})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_project",
		Description: "Fetch one Project by id. Returns destination, optional repo (folder Cursor uses), derived stage, and Decision Map / Spec / Plan summaries.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in projectIDInput) (*mcp.CallToolResult, any, error) {
		project, err := st.GetProject(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(project)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_project",
		Description: "Create a Project parent. Creating a map does not create a Project. Attach a map with create_issue projectId, or move_to_project. Optional repo is the folder send-to-cursor uses for this Project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in createProjectInput) (*mcp.CallToolResult, any, error) {
		project, err := st.CreateProject(store.CreateProject{Title: in.Title, Destination: in.Destination, Repo: in.Repo})
		if err != nil {
			return errResult(err)
		}
		return textResult(project)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_project",
		Description: "Update a Project. Pass title, destination (the product writeup), and/or repo (folder Cursor uses). Empty repo clears it so send-to-cursor falls back to the server default.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in updateProjectInput) (*mcp.CallToolResult, any, error) {
		project, err := st.UpdateProject(in.ID, store.UpdateProject{Title: in.Title, Destination: in.Destination, Repo: in.Repo})
		if err != nil {
			return errResult(err)
		}
		return textResult(project)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "move_to_project",
		Description: "Move issues onto another Project. Pass id to move one issue and its descendants (map tickets, plan tickets). Pass fromProjectId to move every issue currently on that Project (maps, specs, plans, and their children). Destination is projectId. Use this to put a standalone map onto a Project.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in moveToProjectInput) (*mcp.CallToolResult, any, error) {
		result, err := st.MoveToProject(store.MoveToProject{ID: in.ID, FromProjectID: in.FromProjectID, ProjectID: in.ProjectID})
		if err != nil {
			return errResult(err)
		}
		return textResult(result)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_project",
		Description: "Delete a Project and every issue on it (maps, specs, plans, tickets). Remaining issues lose blocked-by edges that pointed at the deleted ids. Empty projects can be deleted. Irreversible.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in projectIDInput) (*mcp.CallToolResult, any, error) {
		result, err := st.DeleteProject(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(result)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "ready_for_spec",
		Description: "Mark a Decision Map ready_for_spec. Explicit lifecycle action; not inferred from closed tickets. Prefer advance_to_spec to mark ready and create the draft in one step.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.ReadyForSpec(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "clear_route",
		Description: "Mark a Decision Map cleared (route is clear). Does not create a spec.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.ClearRoute(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_spec",
		Description: "Create a draft Spec from a map that is already ready_for_spec. Otherwise prefer advance_to_spec. Body is an empty to-spec skeleton (destination copied from the map). Does not run the /to-spec skill.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.CreateSpec(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "advance_to_spec",
		Description: "Mark a Decision Map ready and create its draft Spec in one step. Use this for 'make the spec' when wayfinding is done. Returns the draft spec (empty to-spec skeleton, destination copied from the map); fill the SPEC body via the /to-spec skill, not the map. Does not run the /to-spec skill.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.AdvanceToSpec(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "approve_spec",
		Description: "Approve a draft Spec so Tickets can be created.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.ApproveSpec(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_plan",
		Description: "Create a draft Tickets list from an approved Spec. Does not run /to-tickets. Create each implementation ticket with parentId set to the new Tickets id so it lands in the tickets section.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.CreatePlan(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "advance_to_plan",
		Description: "Create (or return) the Tickets list for a map or spec. From a map: creates that map's spec if needed, approves it, and creates a new Tickets list. Does not inherit another map's spec or tickets. Does not run /to-tickets. Parent each implementation ticket to the returned id.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.AdvanceToPlan(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "activate_plan",
		Description: "Move Tickets from draft to active (project stage implementing).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.ActivatePlan(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "deliver_plan",
		Description: "Mark active Tickets delivered (project stage complete). Explicit; not inferred from zero open tickets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, any, error) {
		issue, err := st.DeliverPlan(in.ID)
		if err != nil {
			return errResult(err)
		}
		return textResult(issue)
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
	Frontier bool     `json:"frontier,omitempty" jsonschema:"if true, only frontier tickets: open, unblocked, unclaimed"`
}

type idInput struct {
	ID int `json:"id" jsonschema:"issue id"`
}

type createInput struct {
	Title       string   `json:"title" jsonschema:"issue title; refer to tickets by this name"`
	Body        string   `json:"body,omitempty" jsonschema:"markdown body"`
	Labels      []string `json:"labels,omitempty"`
	ParentID    *int     `json:"parentId,omitempty" jsonschema:"parent map id for child tickets"`
	LinkedMapID *int     `json:"linkedMapId,omitempty" jsonschema:"existing map; new map joins that map's Project and records an optional linked edge; implies wayfinder:map. Prefer projectId."`
	Project     string   `json:"project,omitempty"`
	ProjectID   *int     `json:"projectId,omitempty" jsonschema:"Project id to attach this issue to; use with labels=[wayfinder:map] to add a Decision Map to a Project"`
	Assignee    string   `json:"assignee,omitempty"`
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

type updateCommentInput struct {
	ID        int    `json:"id" jsonschema:"issue id"`
	CommentID string `json:"commentId" jsonschema:"id of the comment to edit"`
	Body      string `json:"body" jsonschema:"updated markdown body"`
}

type blockedInput struct {
	ID       int   `json:"id" jsonschema:"issue that is blocked"`
	IssueIDs []int `json:"issueIds" jsonschema:"ids of issues that block this one"`
}

type linkedMapsInput struct {
	ID     int   `json:"id" jsonschema:"map issue id"`
	MapIDs []int `json:"mapIds" jsonschema:"ids of maps to link; replaces the current set"`
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

type emptyInput struct{}

type labelInput struct {
	Label string `json:"label" jsonschema:"tag name, without a leading hash"`
}

type addLabelInput struct {
	ID    int    `json:"id" jsonschema:"issue id"`
	Label string `json:"label" jsonschema:"tag to add, without a leading hash"`
}

type wipeInput struct {
	Confirm bool `json:"confirm" jsonschema:"must be true to wipe"`
}

type projectIDInput struct {
	ID int `json:"id" jsonschema:"project id"`
}

type createProjectInput struct {
	Title       string `json:"title" jsonschema:"project title"`
	Destination string `json:"destination,omitempty" jsonschema:"optional destination; otherwise taken from the map body"`
	Repo        string `json:"repo,omitempty" jsonschema:"optional folder Cursor uses for this Project"`
}

type updateProjectInput struct {
	ID          int     `json:"id" jsonschema:"project id"`
	Title       *string `json:"title,omitempty" jsonschema:"new title"`
	Destination *string `json:"destination,omitempty" jsonschema:"product writeup; empty clears"`
	Repo        *string `json:"repo,omitempty" jsonschema:"folder Cursor uses; empty clears"`
}

type moveToProjectInput struct {
	ID            *int `json:"id,omitempty" jsonschema:"issue id to move, including descendants"`
	FromProjectID *int `json:"fromProjectId,omitempty" jsonschema:"move every issue currently on this project"`
	ProjectID     int  `json:"projectId" jsonschema:"destination project id"`
}

type importMapInput struct {
	model.MapBundle
}

type importProjectInput struct {
	model.ProjectBundle
}

func frontierPayload(issues []model.IssueView) map[string]any {
	var next any
	if len(issues) > 0 {
		next = issues[0]
	}
	return map[string]any{"next": next, "issues": issues}
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
