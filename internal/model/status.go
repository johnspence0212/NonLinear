package model

import (
	"fmt"
	"sort"
	"time"
)

const ProjectStatusKind = "nonlinear.project-status"

// TicketCounts is a compact open/closed/frontier split for tickets.
type TicketCounts struct {
	Total    int `json:"total"`
	Open     int `json:"open"`
	Closed   int `json:"closed"`
	Frontier int `json:"frontier"`
	Claimed  int `json:"claimed"`
	Blocked  int `json:"blocked"`
}

// Progress is project-wide artifact and ticket counts.
type Progress struct {
	Maps    int          `json:"maps"`
	Specs   int          `json:"specs"`
	Plans   int          `json:"plans"`
	Tickets TicketCounts `json:"tickets"`
}

// StatusTicket is a ticket without body or comments.
type StatusTicket struct {
	ID           int      `json:"id"`
	Identifier   string   `json:"identifier"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	Labels       []string `json:"labels"`
	Assignee     *string  `json:"assignee,omitempty"`
	ParentID     *int     `json:"parentId,omitempty"`
	Frontier     bool     `json:"frontier"`
	Blocked      bool     `json:"blocked"`
	OpenBlockers int      `json:"openBlockers"`
}

// StatusArtifact is a map, spec, or plan without body or comments.
type StatusArtifact struct {
	ID            int          `json:"id"`
	Identifier    string       `json:"identifier"`
	Title         string       `json:"title"`
	Kind          string       `json:"kind"`
	Lifecycle     string       `json:"lifecycle"`
	State         string       `json:"state"`
	DerivedFromID *int         `json:"derivedFromId,omitempty"`
	Tickets       TicketCounts `json:"tickets"`
}

// NextAction is the suggested MCP move for an agent reading a status snapshot.
type NextAction struct {
	Tool   string `json:"tool,omitempty"`
	ID     *int   `json:"id,omitempty"`
	Reason string `json:"reason"`
}

// ProjectStatus is a big-picture snapshot of a Project. No bodies, no comments.
type ProjectStatus struct {
	Kind        string           `json:"kind"`
	ID          int              `json:"id"`
	Identifier  string           `json:"identifier"`
	Title       string           `json:"title"`
	Stage       string           `json:"stage"`
	Destination string           `json:"destination,omitempty"`
	Repo        string           `json:"repo,omitempty"`
	UpdatedAt   time.Time        `json:"updatedAt"`
	Progress    Progress         `json:"progress"`
	Maps        []StatusArtifact `json:"maps"`
	Specs       []StatusArtifact `json:"specs"`
	Plans       []StatusArtifact `json:"plans"`
	Frontier    []StatusTicket   `json:"frontier"`
	Claimed     []StatusTicket   `json:"claimed"`
	Blocked     []StatusTicket   `json:"blocked"`
	Next        *StatusTicket    `json:"next"`
	NextAction  NextAction       `json:"nextAction"`
}

// BuildProjectStatus assembles a compact status from a Project and the issues on it.
// byID should include every issue in the tracker so blocker edges resolve.
func BuildProjectStatus(project Project, issues []Issue, byID map[int]Issue) ProjectStatus {
	status := ProjectStatus{
		Kind:        ProjectStatusKind,
		ID:          project.ID,
		Identifier:  project.Identifier,
		Title:       project.Title,
		Stage:       DeriveStage(issues),
		Destination: ProjectDestination(project, issues),
		Repo:        project.Repo,
		UpdatedAt:   project.UpdatedAt,
		Maps:        []StatusArtifact{},
		Specs:       []StatusArtifact{},
		Plans:       []StatusArtifact{},
		Frontier:    []StatusTicket{},
		Claimed:     []StatusTicket{},
		Blocked:     []StatusTicket{},
	}
	for _, issue := range issues {
		switch {
		case IsMap(issue):
			status.Maps = append(status.Maps, statusArtifact(issue, issues, byID))
		case IsSpec(issue):
			status.Specs = append(status.Specs, statusArtifact(issue, issues, byID))
		case IsPlan(issue):
			status.Plans = append(status.Plans, statusArtifact(issue, issues, byID))
		default:
			ticket := statusTicket(issue, byID)
			if issue.State == StateOpen {
				if ticket.Frontier {
					status.Frontier = append(status.Frontier, ticket)
				}
				if AssigneeValue(issue) != "" {
					status.Claimed = append(status.Claimed, ticket)
				}
				if ticket.Blocked {
					status.Blocked = append(status.Blocked, ticket)
				}
			}
		}
	}
	sort.Slice(status.Maps, func(i, j int) bool { return status.Maps[i].ID < status.Maps[j].ID })
	sort.Slice(status.Specs, func(i, j int) bool { return status.Specs[i].ID < status.Specs[j].ID })
	sort.Slice(status.Plans, func(i, j int) bool { return status.Plans[i].ID < status.Plans[j].ID })
	sort.Slice(status.Frontier, func(i, j int) bool { return status.Frontier[i].ID < status.Frontier[j].ID })
	sort.Slice(status.Claimed, func(i, j int) bool { return status.Claimed[i].ID < status.Claimed[j].ID })
	sort.Slice(status.Blocked, func(i, j int) bool { return status.Blocked[i].ID < status.Blocked[j].ID })
	status.Progress = Progress{
		Maps:    len(status.Maps),
		Specs:   len(status.Specs),
		Plans:   len(status.Plans),
		Tickets: countTickets(issues, byID, nil),
	}
	if len(status.Frontier) > 0 {
		next := status.Frontier[0]
		status.Next = &next
	}
	status.NextAction = DeriveNextAction(status)
	return status
}

func statusArtifact(issue Issue, issues []Issue, byID map[int]Issue) StatusArtifact {
	kind := issue.Kind
	if kind == "" && IsMap(issue) {
		kind = KindDecisionMap
	}
	id := issue.ID
	return StatusArtifact{
		ID:            issue.ID,
		Identifier:    issue.Identifier,
		Title:         issue.Title,
		Kind:          kind,
		Lifecycle:     issue.Lifecycle,
		State:         issue.State,
		DerivedFromID: issue.DerivedFromArtifactID,
		Tickets:       countTickets(issues, byID, &id),
	}
}

func statusTicket(issue Issue, byID map[int]Issue) StatusTicket {
	labels := append([]string(nil), issue.Labels...)
	if labels == nil {
		labels = []string{}
	}
	return StatusTicket{
		ID:           issue.ID,
		Identifier:   issue.Identifier,
		Title:        issue.Title,
		State:        issue.State,
		Labels:       labels,
		Assignee:     issue.Assignee,
		ParentID:     issue.ParentID,
		Frontier:     IsFrontier(issue, byID),
		Blocked:      IsBlocked(issue, byID),
		OpenBlockers: OpenBlockerCount(issue, byID),
	}
}

func countTickets(issues []Issue, byID map[int]Issue, parent *int) TicketCounts {
	var c TicketCounts
	for _, issue := range issues {
		if IsArtifact(issue) {
			continue
		}
		if parent != nil && (issue.ParentID == nil || *issue.ParentID != *parent) {
			continue
		}
		c.Total++
		if issue.State == StateClosed {
			c.Closed++
			continue
		}
		c.Open++
		if IsFrontier(issue, byID) {
			c.Frontier++
		}
		if AssigneeValue(issue) != "" {
			c.Claimed++
		}
		if IsBlocked(issue, byID) {
			c.Blocked++
		}
	}
	return c
}

// DeriveNextAction picks the next MCP move from a status snapshot.
func DeriveNextAction(status ProjectStatus) NextAction {
	if status.Next != nil {
		id := status.Next.ID
		return NextAction{
			Tool:   "claim_issue",
			ID:     &id,
			Reason: fmt.Sprintf("%s %s is the next frontier ticket", status.Next.Identifier, status.Next.Title),
		}
	}
	if len(status.Claimed) > 0 {
		id := status.Claimed[0].ID
		return NextAction{
			Tool:   "get_issue",
			ID:     &id,
			Reason: fmt.Sprintf("%s %s is claimed and still open", status.Claimed[0].Identifier, status.Claimed[0].Title),
		}
	}
	if len(status.Blocked) > 0 {
		return NextAction{Reason: fmt.Sprintf("%d open ticket(s) are blocked", len(status.Blocked))}
	}
	switch status.Stage {
	case StageWayfinding:
		if len(status.Maps) == 0 {
			return NextAction{Tool: "create_issue", Reason: "project has no decision map"}
		}
		id := status.Maps[0].ID
		return NextAction{
			Tool:   "advance_to_spec",
			ID:     &id,
			Reason: fmt.Sprintf("%s has no frontier tickets", status.Maps[0].Identifier),
		}
	case StageReadyForSpec:
		for _, m := range status.Maps {
			if m.Lifecycle == MapLifecycleReadyForSpec {
				id := m.ID
				return NextAction{Tool: "advance_to_spec", ID: &id, Reason: fmt.Sprintf("%s is ready for spec", m.Identifier)}
			}
		}
		return NextAction{Tool: "advance_to_spec", Reason: "map is ready for spec"}
	case StageSpecReview:
		for _, spec := range status.Specs {
			if spec.Lifecycle == SpecLifecycleDraft {
				id := spec.ID
				return NextAction{Tool: "approve_spec", ID: &id, Reason: fmt.Sprintf("%s is in review", spec.Identifier)}
			}
		}
		return NextAction{Tool: "approve_spec", Reason: "spec is in review"}
	case StageReadyForTickets:
		for _, plan := range status.Plans {
			if plan.Lifecycle == PlanLifecycleDraft {
				id := plan.ID
				return NextAction{Tool: "activate_plan", ID: &id, Reason: fmt.Sprintf("%s is drafted but not started", plan.Identifier)}
			}
		}
		for _, spec := range status.Specs {
			if spec.Lifecycle == SpecLifecycleApproved {
				id := spec.ID
				return NextAction{Tool: "create_plan", ID: &id, Reason: fmt.Sprintf("%s is approved", spec.Identifier)}
			}
		}
		return NextAction{Tool: "create_plan", Reason: "spec is approved"}
	case StageImplementing:
		for _, plan := range status.Plans {
			if plan.Lifecycle == PlanLifecycleActive && plan.Tickets.Open == 0 {
				id := plan.ID
				return NextAction{Tool: "deliver_plan", ID: &id, Reason: fmt.Sprintf("%s has no open tickets", plan.Identifier)}
			}
		}
		return NextAction{Reason: "implementation in progress"}
	case StageComplete:
		return NextAction{Reason: "project is complete"}
	default:
		return NextAction{Reason: "no suggested action"}
	}
}
