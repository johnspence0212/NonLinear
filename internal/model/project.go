package model

import (
	"fmt"
	"strings"
	"time"
)

const SchemaVersion = 1

const (
	KindTicket      = "ticket"
	KindDecisionMap = "decision-map"
	KindSpec        = "spec"
	KindPlan        = "plan"
)

const (
	MapLifecycleActive       = "active"
	MapLifecycleReadyForSpec = "ready_for_spec"
	MapLifecycleCleared      = "cleared"
	MapLifecycleArchived     = "archived"
)

const (
	SpecLifecycleDraft      = "draft"
	SpecLifecycleApproved   = "approved"
	SpecLifecycleSuperseded = "superseded"
)

const (
	PlanLifecycleDraft     = "draft"
	PlanLifecycleActive    = "active"
	PlanLifecycleDelivered = "delivered"
)

const (
	StageWayfinding      = "wayfinding"
	StageReadyForSpec    = "ready_for_spec"
	StageSpecReview      = "spec_review"
	StageReadyForTickets = "ready_for_tickets"
	StageImplementing    = "implementing"
	StageComplete        = "complete"
)

type Project struct {
	ID          int       `json:"id"`
	Identifier  string    `json:"identifier"`
	Title       string    `json:"title"`
	Destination string    `json:"destination,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ProjectSummary struct {
	ID          int    `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Stage       string `json:"stage"`
	Destination string `json:"destination,omitempty"`
}

type ProjectView struct {
	Project
	Stage       string      `json:"stage"`
	Destination string      `json:"destination"`
	Maps        []IssueView `json:"maps"`
	Specs       []IssueView `json:"specs"`
	Plans       []IssueView `json:"plans"`
}

func ProjectIdentifier(id int) string {
	return fmt.Sprintf("P-%d", id)
}

func IsSpec(issue Issue) bool { return issue.Kind == KindSpec }

func IsPlan(issue Issue) bool { return issue.Kind == KindPlan }

func IsArtifact(issue Issue) bool {
	return IsMap(issue) || IsSpec(issue) || IsPlan(issue)
}

func ExtractDestination(body string) string {
	lines := strings.Split(body, "\n")
	in := false
	var b strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimSpace(trimmed[3:])
			if in {
				break
			}
			if strings.EqualFold(heading, "Destination") {
				in = true
				continue
			}
		}
		if in {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

func SpecSkeleton(destination string) string {
	var b strings.Builder
	if strings.TrimSpace(destination) != "" {
		b.WriteString("## Destination\n\n")
		b.WriteString(strings.TrimSpace(destination))
		b.WriteString("\n\n")
	}
	b.WriteString("## Problem Statement\n\n")
	b.WriteString("## Solution\n\n")
	b.WriteString("## User Stories\n\n")
	b.WriteString("## Implementation Decisions\n\n")
	b.WriteString("## Testing Decisions\n\n")
	b.WriteString("## Out of Scope\n\n")
	b.WriteString("## Further Notes\n")
	return b.String()
}

func PlanSkeleton() string {
	return "## Implementation Plan\n\nTickets go on this plan as children. Fill this document via the /to-tickets skill handoff.\n\n## Sequence\n\n## Notes\n"
}

func ProjectDestination(project Project, issues []Issue) string {
	if d := strings.TrimSpace(project.Destination); d != "" {
		return d
	}
	for _, issue := range issues {
		if IsMap(issue) && issueBelongsToProject(issue, project.ID) {
			if d := ExtractDestination(issue.Body); d != "" {
				return d
			}
		}
	}
	return ""
}

func DeriveStage(issues []Issue) string {
	hasSpec := false
	hasSpecDraft := false
	hasSpecApproved := false
	hasPlanActive := false
	hasPlanDelivered := false
	hasMapReady := false
	for _, issue := range issues {
		switch {
		case IsPlan(issue):
			switch issue.Lifecycle {
			case PlanLifecycleDelivered:
				hasPlanDelivered = true
			case PlanLifecycleActive:
				hasPlanActive = true
			}
		case IsSpec(issue):
			hasSpec = true
			switch issue.Lifecycle {
			case SpecLifecycleDraft:
				hasSpecDraft = true
			case SpecLifecycleApproved:
				hasSpecApproved = true
			}
		case IsMap(issue):
			if issue.Lifecycle == MapLifecycleReadyForSpec {
				hasMapReady = true
			}
		}
	}
	switch {
	case hasPlanDelivered:
		return StageComplete
	case hasPlanActive:
		return StageImplementing
	case hasSpecApproved:
		return StageReadyForTickets
	case hasSpecDraft:
		return StageSpecReview
	case hasMapReady && !hasSpec:
		return StageReadyForSpec
	default:
		return StageWayfinding
	}
}

func issueBelongsToProject(issue Issue, projectID int) bool {
	return issue.ProjectID != nil && *issue.ProjectID == projectID
}
