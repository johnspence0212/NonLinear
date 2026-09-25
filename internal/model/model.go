package model

import (
	"sort"
	"time"
)

const (
	StateOpen      = "open"
	StateClosed    = "closed"
	DefaultProject = "inbox"
	DefaultPrefix  = "NL"
)

var SeedLabels = []string{
	"wayfinder:map",
	"wayfinder:research",
	"wayfinder:prototype",
	"wayfinder:grilling",
	"wayfinder:task",
	"needs-triage",
	"needs-info",
	"ready-for-agent",
	"ready-for-human",
	"wontfix",
}

type Comment struct {
	ID        string     `json:"id"`
	Author    string     `json:"author"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

type Issue struct {
	ID                    int       `json:"id"`
	Identifier            string    `json:"identifier"`
	Title                 string    `json:"title"`
	Body                  string    `json:"body"`
	State                 string    `json:"state"`
	Labels                []string  `json:"labels"`
	Assignee              *string   `json:"assignee"`
	ParentID              *int      `json:"parentId"`
	BlockedBy             []int     `json:"blockedBy"`
	LinkedMaps            []int     `json:"linkedMaps"`
	Project               string    `json:"project"`
	Kind                  string    `json:"kind,omitempty"`
	ProjectID             *int      `json:"projectId,omitempty"`
	Lifecycle             string    `json:"lifecycle,omitempty"`
	DerivedFromArtifactID *int      `json:"derivedFromArtifactId,omitempty"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
	Comments              []Comment `json:"comments"`
}

type IssueView struct {
	Issue
	Frontier     bool            `json:"frontier"`
	Blocked      bool            `json:"blocked"`
	OpenBlockers int             `json:"openBlockers"`
	Children     []IssueSummary  `json:"children,omitempty"`
	Blockers     []IssueSummary  `json:"blockers,omitempty"`
	Blocks       []IssueSummary  `json:"blocks,omitempty"`
	Linked       []IssueSummary  `json:"linked,omitempty"`
	Parent       *IssueSummary   `json:"parent,omitempty"`
	DerivedFrom  *IssueSummary   `json:"derivedFrom,omitempty"`
	Derived      []IssueSummary  `json:"derived,omitempty"`
	ProjectRef   *ProjectSummary `json:"projectRef,omitempty"`
}

type IssueSummary struct {
	ID           int      `json:"id"`
	Identifier   string   `json:"identifier"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	Kind         string   `json:"kind,omitempty"`
	Lifecycle    string   `json:"lifecycle,omitempty"`
	Labels       []string `json:"labels"`
	Assignee     *string  `json:"assignee"`
	Frontier     bool     `json:"frontier"`
	Blocked      bool     `json:"blocked"`
	OpenBlockers int      `json:"openBlockers"`
}

type DB struct {
	SchemaVersion int       `json:"schemaVersion,omitempty"`
	NextID        int       `json:"nextId"`
	NextProjectID int       `json:"nextProjectId"`
	Prefix        string    `json:"prefix"`
	Issues        []Issue   `json:"issues"`
	Labels        []string  `json:"labels,omitempty"`
	Projects      []Project `json:"projects"`
	Events        []Event   `json:"events,omitempty"`
}

func CloneIssue(in Issue) Issue {
	out := in
	out.Labels = append([]string(nil), in.Labels...)
	out.BlockedBy = append([]int(nil), in.BlockedBy...)
	out.LinkedMaps = append([]int(nil), in.LinkedMaps...)
	out.Comments = append([]Comment(nil), in.Comments...)
	if in.Assignee != nil {
		v := *in.Assignee
		out.Assignee = &v
	}
	if in.ParentID != nil {
		v := *in.ParentID
		out.ParentID = &v
	}
	if in.ProjectID != nil {
		v := *in.ProjectID
		out.ProjectID = &v
	}
	if in.DerivedFromArtifactID != nil {
		v := *in.DerivedFromArtifactID
		out.DerivedFromArtifactID = &v
	}
	if out.Labels == nil {
		out.Labels = []string{}
	}
	if out.BlockedBy == nil {
		out.BlockedBy = []int{}
	}
	if out.LinkedMaps == nil {
		out.LinkedMaps = []int{}
	}
	if out.Comments == nil {
		out.Comments = []Comment{}
	}
	return out
}

func HasLabel(issue Issue, label string) bool {
	for _, l := range issue.Labels {
		if l == label {
			return true
		}
	}
	return false
}

func AssigneeValue(issue Issue) string {
	if issue.Assignee == nil {
		return ""
	}
	return *issue.Assignee
}

func OpenBlockerCount(issue Issue, byID map[int]Issue) int {
	n := 0
	for _, id := range issue.BlockedBy {
		blocker, ok := byID[id]
		if !ok || blocker.State != StateClosed {
			n++
		}
	}
	return n
}

func IsBlocked(issue Issue, byID map[int]Issue) bool {
	return OpenBlockerCount(issue, byID) > 0
}

func IsMap(issue Issue) bool {
	return issue.Kind == KindDecisionMap || HasLabel(issue, "wayfinder:map")
}

func IsFrontier(issue Issue, byID map[int]Issue) bool {
	if issue.State != StateOpen {
		return false
	}
	if IsMap(issue) || IsSpec(issue) || IsPlan(issue) {
		return false
	}
	if AssigneeValue(issue) != "" {
		return false
	}
	return !IsBlocked(issue, byID)
}

func Summarize(issue Issue, byID map[int]Issue) IssueSummary {
	return IssueSummary{
		ID:           issue.ID,
		Identifier:   issue.Identifier,
		Title:        issue.Title,
		State:        issue.State,
		Kind:         issue.Kind,
		Lifecycle:    issue.Lifecycle,
		Labels:       append([]string(nil), issue.Labels...),
		Assignee:     issue.Assignee,
		Frontier:     IsFrontier(issue, byID),
		Blocked:      IsBlocked(issue, byID),
		OpenBlockers: OpenBlockerCount(issue, byID),
	}
}

func isSupersededSpec(issue Issue) bool {
	return IsSpec(issue) && issue.Lifecycle == SpecLifecycleSuperseded
}

func View(issue Issue, byID map[int]Issue) IssueView {
	v := IssueView{
		Issue:        CloneIssue(issue),
		Frontier:     IsFrontier(issue, byID),
		Blocked:      IsBlocked(issue, byID),
		OpenBlockers: OpenBlockerCount(issue, byID),
		Children:     []IssueSummary{},
		Blockers:     []IssueSummary{},
		Blocks:       []IssueSummary{},
		Linked:       []IssueSummary{},
		Derived:      []IssueSummary{},
	}
	if issue.ParentID != nil {
		if parent, ok := byID[*issue.ParentID]; ok {
			s := Summarize(parent, byID)
			v.Parent = &s
		}
	}
	if issue.DerivedFromArtifactID != nil {
		if src, ok := byID[*issue.DerivedFromArtifactID]; ok {
			s := Summarize(src, byID)
			v.DerivedFrom = &s
		}
	}
	for _, other := range byID {
		if other.ParentID != nil && *other.ParentID == issue.ID && !IsMap(other) {
			v.Children = append(v.Children, Summarize(other, byID))
		}
		if other.DerivedFromArtifactID != nil && *other.DerivedFromArtifactID == issue.ID && !isSupersededSpec(other) {
			v.Derived = append(v.Derived, Summarize(other, byID))
		}
		for _, bid := range other.BlockedBy {
			if bid == issue.ID {
				v.Blocks = append(v.Blocks, Summarize(other, byID))
				break
			}
		}
	}
	sort.Slice(v.Children, func(i, j int) bool { return v.Children[i].ID < v.Children[j].ID })
	sort.Slice(v.Derived, func(i, j int) bool { return v.Derived[i].ID < v.Derived[j].ID })
	sort.Slice(v.Blocks, func(i, j int) bool { return v.Blocks[i].ID < v.Blocks[j].ID })
	for _, id := range issue.BlockedBy {
		if blocker, ok := byID[id]; ok {
			v.Blockers = append(v.Blockers, Summarize(blocker, byID))
		}
	}
	seenLinked := map[int]bool{}
	addLinked := func(id int) {
		if id == issue.ID || seenLinked[id] {
			return
		}
		other, ok := byID[id]
		if !ok || !IsMap(other) {
			return
		}
		seenLinked[id] = true
		v.Linked = append(v.Linked, Summarize(other, byID))
	}
	for _, id := range issue.LinkedMaps {
		addLinked(id)
	}
	for _, other := range byID {
		for _, id := range other.LinkedMaps {
			if id == issue.ID {
				addLinked(other.ID)
				break
			}
		}
	}
	sort.Slice(v.Linked, func(i, j int) bool { return v.Linked[i].ID < v.Linked[j].ID })
	return v
}
