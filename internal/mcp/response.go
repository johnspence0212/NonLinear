package mcpserver

import "github.com/johnspence0212/NonLinear/internal/model"

// issueAck is the compact write confirmation for issue mutations.
// Bodies, comments, children, parent objects, and project snapshots are omitted.
type issueAck struct {
	ID                    int      `json:"id"`
	Identifier            string   `json:"identifier"`
	State                 string   `json:"state"`
	Updated               bool     `json:"updated"`
	Created               bool     `json:"created,omitempty"`
	Title                 string   `json:"title,omitempty"`
	Kind                  string   `json:"kind,omitempty"`
	Lifecycle             string   `json:"lifecycle,omitempty"`
	Labels                []string `json:"labels,omitempty"`
	Assignee              *string  `json:"assignee,omitempty"`
	ParentID              *int     `json:"parentId,omitempty"`
	ProjectID             *int     `json:"projectId,omitempty"`
	DerivedFromArtifactID *int     `json:"derivedFromArtifactId,omitempty"`
	BlockedBy             []int    `json:"blockedBy"`
	LinkedMaps            *[]int   `json:"linkedMaps,omitempty"`
	Frontier              bool     `json:"frontier"`
	Blocked               bool     `json:"blocked"`
	OpenBlockers          int      `json:"openBlockers"`
	CommentID             string   `json:"commentId,omitempty"`
	CommentCount          int      `json:"commentCount,omitempty"`
}

// issueSummary is the compact list/frontier row: state plus navigation, no bodies.
type issueSummary struct {
	ID           int      `json:"id"`
	Identifier   string   `json:"identifier"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	Kind         string   `json:"kind,omitempty"`
	Lifecycle    string   `json:"lifecycle,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	Assignee     *string  `json:"assignee,omitempty"`
	ParentID     *int     `json:"parentId,omitempty"`
	ProjectID    *int     `json:"projectId,omitempty"`
	BlockedBy    []int    `json:"blockedBy,omitempty"`
	Frontier     bool     `json:"frontier"`
	Blocked      bool     `json:"blocked"`
	OpenBlockers int      `json:"openBlockers"`
}

type projectAck struct {
	ID          int    `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Stage       string `json:"stage"`
	Destination string `json:"destination,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Updated     bool   `json:"updated"`
	Created     bool   `json:"created,omitempty"`
}

type projectSummary struct {
	ID          int            `json:"id"`
	Identifier  string         `json:"identifier"`
	Title       string         `json:"title"`
	Stage       string         `json:"stage"`
	Destination string         `json:"destination,omitempty"`
	Repo        string         `json:"repo,omitempty"`
	Maps        []issueSummary `json:"maps"`
	Specs       []issueSummary `json:"specs"`
	Plans       []issueSummary `json:"plans"`
}

func ackIssue(v model.IssueView, created bool) issueAck {
	out := issueAck{
		ID:                    v.ID,
		Identifier:            v.Identifier,
		State:                 v.State,
		Updated:               true,
		Created:               created,
		Title:                 v.Title,
		Lifecycle:             v.Lifecycle,
		Labels:                append([]string(nil), v.Labels...),
		Assignee:              v.Assignee,
		ParentID:              v.ParentID,
		ProjectID:             v.ProjectID,
		DerivedFromArtifactID: v.DerivedFromArtifactID,
		BlockedBy:             append([]int(nil), v.BlockedBy...),
		Frontier:              v.Frontier,
		Blocked:               v.Blocked,
		OpenBlockers:          v.OpenBlockers,
	}
	if out.BlockedBy == nil {
		out.BlockedBy = []int{}
	}
	if model.IsArtifact(v.Issue) {
		out.Kind = model.ArtifactKind(v.Issue)
	}
	if len(v.LinkedMaps) > 0 {
		ids := append([]int(nil), v.LinkedMaps...)
		out.LinkedMaps = &ids
	}
	return out
}

func ackComment(v model.IssueView, commentID string) issueAck {
	out := ackIssue(v, false)
	out.CommentID = commentID
	out.CommentCount = len(v.Comments)
	return out
}

func ackLinked(v model.IssueView) issueAck {
	out := ackIssue(v, false)
	ids := append([]int(nil), v.LinkedMaps...)
	if ids == nil {
		ids = []int{}
	}
	out.LinkedMaps = &ids
	return out
}

func lastCommentID(v model.IssueView) string {
	if n := len(v.Comments); n > 0 {
		return v.Comments[n-1].ID
	}
	return ""
}

func summarizeIssue(v model.IssueView) issueSummary {
	out := issueSummary{
		ID:           v.ID,
		Identifier:   v.Identifier,
		Title:        v.Title,
		State:        v.State,
		Lifecycle:    v.Lifecycle,
		Labels:       append([]string(nil), v.Labels...),
		Assignee:     v.Assignee,
		ParentID:     v.ParentID,
		ProjectID:    v.ProjectID,
		BlockedBy:    append([]int(nil), v.BlockedBy...),
		Frontier:     v.Frontier,
		Blocked:      v.Blocked,
		OpenBlockers: v.OpenBlockers,
	}
	if model.IsArtifact(v.Issue) {
		out.Kind = model.ArtifactKind(v.Issue)
	}
	return out
}

func summarizeIssues(vs []model.IssueView) []issueSummary {
	out := make([]issueSummary, 0, len(vs))
	for _, v := range vs {
		out = append(out, summarizeIssue(v))
	}
	return out
}

func summarizeProject(v model.ProjectView) projectSummary {
	return projectSummary{
		ID:          v.ID,
		Identifier:  v.Identifier,
		Title:       v.Title,
		Stage:       v.Stage,
		Destination: v.Destination,
		Repo:        v.Repo,
		Maps:        summarizeIssues(v.Maps),
		Specs:       summarizeIssues(v.Specs),
		Plans:       summarizeIssues(v.Plans),
	}
}

func summarizeProjects(vs []model.ProjectView) []projectSummary {
	out := make([]projectSummary, 0, len(vs))
	for _, v := range vs {
		out = append(out, summarizeProject(v))
	}
	return out
}

func ackProject(v model.ProjectView, created bool) projectAck {
	return projectAck{
		ID:          v.ID,
		Identifier:  v.Identifier,
		Title:       v.Title,
		Stage:       v.Stage,
		Destination: v.Destination,
		Repo:        v.Repo,
		Updated:     true,
		Created:     created,
	}
}
