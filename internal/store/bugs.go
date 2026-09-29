package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/johnspence0212/NonLinear/internal/model"
)

type CreateBug struct {
	Title     string
	Body      string
	ProjectID int
}

func (s *Store) CreateBug(in CreateBug) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createBugLocked(in)
}

func (s *Store) createBugLocked(in CreateBug) (model.IssueView, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return model.IssueView{}, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if s.findProjectLocked(in.ProjectID) == nil {
		return model.IssueView{}, fmt.Errorf("%w: project %d", ErrNotFound, in.ProjectID)
	}
	if s.db.NextBugID < 1 {
		s.db.NextBugID = 1
	}
	now := time.Now().UTC()
	id := s.db.NextID
	s.db.NextID++
	bugN := s.db.NextBugID
	s.db.NextBugID++
	pid := in.ProjectID
	issue := model.Issue{
		ID:         id,
		Identifier: model.BugIdentifier(bugN),
		Title:      title,
		Body:       in.Body,
		State:      model.StateOpen,
		Labels:     []string{},
		BlockedBy:  []int{},
		LinkedMaps: []int{},
		Project:    model.DefaultProject,
		Kind:       model.KindBug,
		ProjectID:  &pid,
		CreatedAt:  now,
		UpdatedAt:  now,
		Comments:   []model.Comment{},
	}
	s.db.Issues = append(s.db.Issues, issue)
	s.appendEventLocked(s.eventFromIssueLocked(model.EventCreated, "cursor", "", issue))
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(s.byIDLocked()[id]), nil
}

func (s *Store) ListBugs(projectID int, state string) []model.IssueView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listBugsLocked(projectID, state)
}

func (s *Store) listBugsLocked(projectID int, state string) []model.IssueView {
	out := []model.IssueView{}
	state = strings.TrimSpace(state)
	for _, issue := range s.db.Issues {
		if !model.IsBug(issue) {
			continue
		}
		if issue.ProjectID == nil || *issue.ProjectID != projectID {
			continue
		}
		if state != "" && issue.State != state {
			continue
		}
		out = append(out, s.viewLocked(issue))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
