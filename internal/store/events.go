package store

import (
	"sort"
	"strings"
	"time"

	"github.com/johnspence0212/NonLinear/internal/model"
)

func (s *Store) Home() model.HomeView {
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := s.byIDLocked()
	claimed := []model.IssueSummary{}
	frontier := []model.IssueSummary{}
	blocked := []model.IssueSummary{}
	for _, issue := range s.db.Issues {
		if model.IsArtifact(issue) || issue.State != model.StateOpen {
			continue
		}
		sum := model.Summarize(issue, byID)
		if sum.Frontier {
			frontier = append(frontier, sum)
		}
		if model.AssigneeValue(issue) != "" {
			claimed = append(claimed, sum)
		}
		if sum.Blocked {
			blocked = append(blocked, sum)
		}
	}
	sortSummaries(claimed, byID)
	sortSummaries(frontier, byID)
	sortSummaries(blocked, byID)
	events := append([]model.Event(nil), s.db.Events...)
	if events == nil {
		events = []model.Event{}
	}
	return model.HomeView{
		Claimed:  claimed,
		Frontier: frontier,
		Blocked:  blocked,
		Events:   events,
		Focus:    s.focusProjectLocked(events),
		Today:    countToday(events, time.Now().UTC()),
	}
}

func sortSummaries(items []model.IssueSummary, byID map[int]model.Issue) {
	sort.Slice(items, func(i, j int) bool {
		ai := byID[items[i].ID]
		aj := byID[items[j].ID]
		if !ai.UpdatedAt.Equal(aj.UpdatedAt) {
			return ai.UpdatedAt.After(aj.UpdatedAt)
		}
		return items[i].ID > items[j].ID
	})
}

func (s *Store) focusProjectLocked(events []model.Event) *model.ProjectSummary {
	for _, ev := range events {
		if ev.ProjectID == nil {
			continue
		}
		p := s.findProjectLocked(*ev.ProjectID)
		if p == nil {
			continue
		}
		sum := model.ProjectSummary{
			ID:          p.ID,
			Identifier:  p.Identifier,
			Title:       p.Title,
			Stage:       model.DeriveStage(s.issuesForProjectLocked(p.ID)),
			Destination: model.ProjectDestination(*p, s.db.Issues),
			Repo:        p.Repo,
		}
		return &sum
	}
	return nil
}

func countToday(events []model.Event, now time.Time) model.HomeToday {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var t model.HomeToday
	for _, ev := range events {
		if ev.At.Before(start) {
			continue
		}
		switch ev.Kind {
		case model.EventResolved, model.EventClosed:
			t.Resolved++
		case model.EventClaimed:
			t.Claimed++
		case model.EventCreated:
			t.Created++
		default:
			if model.IsLifecycleEvent(ev.Kind) {
				t.Lifecycle++
			}
		}
	}
	return t
}

func (s *Store) appendEventLocked(ev model.Event) {
	if s.quietEvents {
		return
	}
	if ev.Actor == "" {
		ev.Actor = "cursor"
	}
	if ev.ID == "" {
		ev.ID = newID()
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	s.db.Events = append([]model.Event{ev}, s.db.Events...)
	if len(s.db.Events) > model.MaxEvents {
		s.db.Events = s.db.Events[:model.MaxEvents]
	}
}

func (s *Store) eventFromIssueLocked(kind, actor, gist string, issue model.Issue) model.Event {
	ev := model.Event{
		Actor:      actor,
		Kind:       kind,
		Identifier: issue.Identifier,
		Title:      issue.Title,
		Gist:       gist,
		IssueID:    &issue.ID,
		ProjectID:  cloneInt(issue.ProjectID),
	}
	if issue.ProjectID != nil {
		if p := s.findProjectLocked(*issue.ProjectID); p != nil {
			ev.ProjectRef = p.Identifier
		}
	}
	switch {
	case model.IsMap(issue):
		ev.TargetKind = "map"
	case model.IsSpec(issue):
		ev.TargetKind = "spec"
	case model.IsPlan(issue):
		ev.TargetKind = "plan"
	default:
		ev.TargetKind = "issue"
	}
	return ev
}

func (s *Store) recordIssueDeltaLocked(old, issue model.Issue) {
	if s.quietEvents {
		return
	}
	if model.AssigneeValue(old) == "" && model.AssigneeValue(issue) != "" {
		s.appendEventLocked(s.eventFromIssueLocked(model.EventClaimed, model.AssigneeValue(issue), "", issue))
	}
	if model.AssigneeValue(old) != "" && model.AssigneeValue(issue) == "" {
		s.appendEventLocked(s.eventFromIssueLocked(model.EventUnclaimed, "cursor", "", issue))
	}
	if old.State != model.StateClosed && issue.State == model.StateClosed {
		s.appendEventLocked(s.eventFromIssueLocked(model.EventClosed, "cursor", "", issue))
	}
	if old.State == model.StateClosed && issue.State != model.StateClosed {
		s.appendEventLocked(s.eventFromIssueLocked(model.EventReopened, "cursor", "", issue))
	}
}

func (s *Store) setQuiet(on bool) {
	s.mu.Lock()
	s.quietEvents = on
	s.mu.Unlock()
}

func (s *Store) recordUnlocked(ev model.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quietEvents = false
	s.appendEventLocked(ev)
	_ = s.saveLocked()
}

func gistLine(body string) string {
	line := strings.TrimSpace(strings.SplitN(body, "\n", 2)[0])
	if len(line) > 120 {
		return line[:117] + "…"
	}
	return line
}
