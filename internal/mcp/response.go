package mcpserver

import "github.com/johnspence0212/NonLinear/internal/model"

// compactIssue is the MCP list row and write confirmation. Bodies, comments,
// children, and parent/project snapshots are omitted; get_issue still returns those.
type compactIssue struct {
	ID                    int      `json:"id"`
	Identifier            string   `json:"identifier"`
	Title                 string   `json:"title,omitempty"`
	State                 string   `json:"state"`
	Kind                  string   `json:"kind,omitempty"`
	Lifecycle             string   `json:"lifecycle,omitempty"`
	Labels                []string `json:"labels,omitempty"`
	Assignee              *string  `json:"assignee,omitempty"`
	ParentID              *int     `json:"parentId,omitempty"`
	ProjectID             *int     `json:"projectId,omitempty"`
	DerivedFromArtifactID *int     `json:"derivedFromArtifactId,omitempty"`
	BlockedBy             []int    `json:"blockedBy"`
	LinkedMaps            []int    `json:"linkedMaps,omitempty"`
	Frontier              bool     `json:"frontier"`
	Blocked               bool     `json:"blocked"`
	OpenBlockers          int      `json:"openBlockers"`
	Updated               bool     `json:"updated,omitempty"`
	Created               bool     `json:"created,omitempty"`
	CommentID             string   `json:"commentId,omitempty"`
}

type compactProject struct {
	ID          int            `json:"id"`
	Identifier  string         `json:"identifier"`
	Title       string         `json:"title"`
	Stage       string         `json:"stage"`
	Destination string         `json:"destination,omitempty"`
	Repo        string         `json:"repo,omitempty"`
	Updated     bool           `json:"updated,omitempty"`
	Created     bool           `json:"created,omitempty"`
	Maps        []compactIssue `json:"maps,omitempty"`
	Specs       []compactIssue `json:"specs,omitempty"`
	Plans       []compactIssue `json:"plans,omitempty"`
}

func compact(v model.IssueView) compactIssue {
	out := compactIssue{
		ID:                    v.ID,
		Identifier:            v.Identifier,
		Title:                 v.Title,
		State:                 v.State,
		Lifecycle:             v.Lifecycle,
		Labels:                v.Labels,
		Assignee:              v.Assignee,
		ParentID:              v.ParentID,
		ProjectID:             v.ProjectID,
		DerivedFromArtifactID: v.DerivedFromArtifactID,
		BlockedBy:             v.BlockedBy,
		LinkedMaps:            v.LinkedMaps,
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
	return out
}

func compactAll(vs []model.IssueView) []compactIssue {
	out := make([]compactIssue, len(vs))
	for i, v := range vs {
		out[i] = compact(v)
	}
	return out
}

func writeIssue(v model.IssueView, created bool) compactIssue {
	out := compact(v)
	out.Updated = true
	out.Created = created
	return out
}

func writeProject(v model.ProjectView, created bool) compactProject {
	return compactProject{
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

func compactProjects(vs []model.ProjectView) []compactProject {
	out := make([]compactProject, len(vs))
	for i, v := range vs {
		out[i] = compactProject{
			ID:          v.ID,
			Identifier:  v.Identifier,
			Title:       v.Title,
			Stage:       v.Stage,
			Destination: v.Destination,
			Repo:        v.Repo,
			Maps:        compactAll(v.Maps),
			Specs:       compactAll(v.Specs),
			Plans:       compactAll(v.Plans),
		}
	}
	return out
}
