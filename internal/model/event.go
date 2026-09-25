package model

import "time"

const (
	EventCreated       = "created"
	EventClaimed       = "claimed"
	EventUnclaimed     = "unclaimed"
	EventResolved      = "resolved"
	EventClosed        = "closed"
	EventReopened      = "reopened"
	EventCommented     = "commented"
	EventBlocked       = "blocked"
	EventReadyForSpec  = "ready_for_spec"
	EventCleared       = "cleared"
	EventSpecDraft     = "spec_draft"
	EventSpecApproved  = "spec_approved"
	EventPlanDraft     = "plan_draft"
	EventPlanActive    = "plan_active"
	EventPlanDelivered = "plan_delivered"
	EventDeleted       = "deleted"
)

const MaxEvents = 200

type Event struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	Actor      string    `json:"actor"`
	Kind       string    `json:"kind"`
	Identifier string    `json:"identifier,omitempty"`
	Title      string    `json:"title,omitempty"`
	Gist       string    `json:"gist,omitempty"`
	IssueID    *int      `json:"issueId,omitempty"`
	ProjectID  *int      `json:"projectId,omitempty"`
	ProjectRef string    `json:"projectRef,omitempty"`
	TargetKind string    `json:"targetKind,omitempty"`
}

type HomeToday struct {
	Resolved  int `json:"resolved"`
	Claimed   int `json:"claimed"`
	Created   int `json:"created"`
	Lifecycle int `json:"lifecycle"`
}

type HomeView struct {
	Claimed  []IssueSummary  `json:"claimed"`
	Frontier []IssueSummary  `json:"frontier"`
	Blocked  []IssueSummary  `json:"blocked"`
	Events   []Event         `json:"events"`
	Focus    *ProjectSummary `json:"focus,omitempty"`
	Today    HomeToday       `json:"today"`
}

func EventDot(kind string) string {
	switch kind {
	case EventClaimed, EventResolved, EventClosed, EventReopened:
		return "work"
	case EventReadyForSpec, EventCleared, EventSpecDraft, EventSpecApproved, EventPlanDraft, EventPlanActive, EventPlanDelivered, EventBlocked:
		return "life"
	case EventCommented:
		return "note"
	default:
		return "create"
	}
}

func EventStamp(kind string) string {
	switch kind {
	case EventReadyForSpec:
		return "ready_for_spec"
	case EventSpecDraft:
		return "spec"
	case EventSpecApproved:
		return "approved"
	case EventPlanDraft:
		return "tickets"
	case EventPlanActive:
		return "active"
	case EventPlanDelivered:
		return "delivered"
	default:
		return kind
	}
}

func IsLifecycleEvent(kind string) bool {
	switch kind {
	case EventReadyForSpec, EventCleared, EventSpecDraft, EventSpecApproved, EventPlanDraft, EventPlanActive, EventPlanDelivered, EventBlocked:
		return true
	default:
		return false
	}
}
