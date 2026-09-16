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
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type Issue struct {
	ID         int       `json:"id"`
	Identifier string    `json:"identifier"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	State      string    `json:"state"`
	Labels     []string  `json:"labels"`
	Assignee   *string   `json:"assignee"`
	ParentID   *int      `json:"parentId"`
	BlockedBy  []int     `json:"blockedBy"`
	Project    string    `json:"project"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Comments   []Comment `json:"comments"`
}

type IssueView struct {
	Issue
	Frontier     bool           `json:"frontier"`
	Blocked      bool           `json:"blocked"`
	OpenBlockers int            `json:"openBlockers"`
	Children     []IssueSummary `json:"children,omitempty"`
	Blockers     []IssueSummary `json:"blockers,omitempty"`
	Parent       *IssueSummary  `json:"parent,omitempty"`
}

type IssueSummary struct {
	ID           int      `json:"id"`
	Identifier   string   `json:"identifier"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	Labels       []string `json:"labels"`
	Assignee     *string  `json:"assignee"`
	Frontier     bool     `json:"frontier"`
	Blocked      bool     `json:"blocked"`
	OpenBlockers int      `json:"openBlockers"`
}

type DB struct {
	NextID int     `json:"nextId"`
	Prefix string  `json:"prefix"`
	Issues []Issue `json:"issues"`
}

func CloneIssue(in Issue) Issue {
	out := in
	out.Labels = append([]string(nil), in.Labels...)
	out.BlockedBy = append([]int(nil), in.BlockedBy...)
	out.Comments = append([]Comment(nil), in.Comments...)
	if in.Assignee != nil {
		v := *in.Assignee
		out.Assignee = &v
	}
	if in.ParentID != nil {
		v := *in.ParentID
		out.ParentID = &v
	}
	if out.Labels == nil {
		out.Labels = []string{}
	}
	if out.BlockedBy == nil {
		out.BlockedBy = []int{}
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

func IsFrontier(issue Issue, byID map[int]Issue) bool {
	if issue.State != StateOpen {
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
		Labels:       append([]string(nil), issue.Labels...),
		Assignee:     issue.Assignee,
		Frontier:     IsFrontier(issue, byID),
		Blocked:      IsBlocked(issue, byID),
		OpenBlockers: OpenBlockerCount(issue, byID),
	}
}

func View(issue Issue, byID map[int]Issue) IssueView {
	v := IssueView{
		Issue:        CloneIssue(issue),
		Frontier:     IsFrontier(issue, byID),
		Blocked:      IsBlocked(issue, byID),
		OpenBlockers: OpenBlockerCount(issue, byID),
		Children:     []IssueSummary{},
		Blockers:     []IssueSummary{},
	}
	if issue.ParentID != nil {
		if parent, ok := byID[*issue.ParentID]; ok {
			s := Summarize(parent, byID)
			v.Parent = &s
		}
	}
	for _, other := range byID {
		if other.ParentID != nil && *other.ParentID == issue.ID {
			v.Children = append(v.Children, Summarize(other, byID))
		}
	}
	sort.Slice(v.Children, func(i, j int) bool { return v.Children[i].ID < v.Children[j].ID })
	for _, id := range issue.BlockedBy {
		if blocker, ok := byID[id]; ok {
			v.Blockers = append(v.Blockers, Summarize(blocker, byID))
		}
	}
	return v
}
