package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/johnspence0212/NonLinear/internal/model"
)

type CreateProject struct {
	Title       string
	Destination string
}

func (s *Store) viewLocked(issue model.Issue) model.IssueView {
	v := model.View(issue, s.byIDLocked())
	if issue.ProjectID == nil {
		return v
	}
	p := s.findProjectLocked(*issue.ProjectID)
	if p == nil {
		return v
	}
	summary := model.ProjectSummary{
		ID:          p.ID,
		Identifier:  p.Identifier,
		Title:       p.Title,
		Stage:       model.DeriveStage(s.issuesForProjectLocked(p.ID)),
		Destination: model.ProjectDestination(*p, s.db.Issues),
	}
	v.ProjectRef = &summary
	return v
}

func (s *Store) findProjectLocked(id int) *model.Project {
	for i := range s.db.Projects {
		if s.db.Projects[i].ID == id {
			return &s.db.Projects[i]
		}
	}
	return nil
}

func (s *Store) issuesForProjectLocked(id int) []model.Issue {
	out := []model.Issue{}
	for _, issue := range s.db.Issues {
		if issue.ProjectID != nil && *issue.ProjectID == id {
			out = append(out, issue)
		}
	}
	return out
}

func (s *Store) ensureMapLocked(idx int) {
	issue := &s.db.Issues[idx]
	if !model.IsMap(*issue) {
		return
	}
	if issue.Kind == "" {
		issue.Kind = model.KindDecisionMap
	}
	if issue.Lifecycle == "" {
		issue.Lifecycle = model.MapLifecycleActive
	}
}

func (s *Store) projectViewLocked(p model.Project) model.ProjectView {
	issues := s.issuesForProjectLocked(p.ID)
	view := model.ProjectView{
		Project:     p,
		Stage:       model.DeriveStage(issues),
		Destination: model.ProjectDestination(p, issues),
		Maps:        []model.IssueView{},
		Specs:       []model.IssueView{},
		Plans:       []model.IssueView{},
	}
	for _, issue := range issues {
		item := s.viewLocked(issue)
		switch {
		case model.IsMap(issue):
			view.Maps = append(view.Maps, item)
		case model.IsSpec(issue):
			view.Specs = append(view.Specs, item)
		case model.IsPlan(issue):
			view.Plans = append(view.Plans, item)
		}
	}
	sort.Slice(view.Maps, func(i, j int) bool { return view.Maps[i].ID < view.Maps[j].ID })
	sort.Slice(view.Specs, func(i, j int) bool { return view.Specs[i].ID < view.Specs[j].ID })
	sort.Slice(view.Plans, func(i, j int) bool { return view.Plans[i].ID < view.Plans[j].ID })
	return view
}

func (s *Store) ListProjects() []model.ProjectView {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.ProjectView, 0, len(s.db.Projects))
	for _, p := range s.db.Projects {
		out = append(out, s.projectViewLocked(p))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Store) GetProject(id int) (model.ProjectView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.findProjectLocked(id)
	if p == nil {
		return model.ProjectView{}, fmt.Errorf("%w: project %d", ErrNotFound, id)
	}
	return s.projectViewLocked(*p), nil
}

func (s *Store) CreateProject(in CreateProject) (model.ProjectView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return model.ProjectView{}, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if s.db.NextProjectID < 1 {
		s.db.NextProjectID = 1
	}
	now := time.Now().UTC()
	id := s.db.NextProjectID
	s.db.NextProjectID++
	p := model.Project{
		ID:          id,
		Identifier:  model.ProjectIdentifier(id),
		Title:       title,
		Destination: strings.TrimSpace(in.Destination),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.db.Projects = append(s.db.Projects, p)
	if err := s.saveLocked(); err != nil {
		return model.ProjectView{}, err
	}
	return s.projectViewLocked(p), nil
}

type MoveToProject struct {
	ID            *int
	FromProjectID *int
	ProjectID     int
}

type MoveResult struct {
	Moved   []int             `json:"moved"`
	Project model.ProjectView `json:"project"`
}

func (s *Store) MoveToProject(in MoveToProject) (MoveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dest := s.findProjectLocked(in.ProjectID)
	if dest == nil {
		return MoveResult{}, fmt.Errorf("%w: project %d", ErrNotFound, in.ProjectID)
	}
	if in.ID == nil && in.FromProjectID == nil {
		return MoveResult{}, fmt.Errorf("%w: id or fromProjectId is required", ErrInvalid)
	}
	ids := map[int]bool{}
	if in.ID != nil {
		if _, ok := s.findLocked(*in.ID); !ok {
			return MoveResult{}, ErrNotFound
		}
		for _, did := range s.descendantsLocked(*in.ID) {
			ids[did] = true
		}
	}
	if in.FromProjectID != nil {
		if s.findProjectLocked(*in.FromProjectID) == nil {
			return MoveResult{}, fmt.Errorf("%w: project %d", ErrNotFound, *in.FromProjectID)
		}
		for _, issue := range s.issuesForProjectLocked(*in.FromProjectID) {
			for _, did := range s.descendantsLocked(issue.ID) {
				ids[did] = true
			}
		}
	}
	now := time.Now().UTC()
	pid := dest.ID
	if in.FromProjectID != nil {
		if src := s.findProjectLocked(*in.FromProjectID); src != nil && strings.TrimSpace(dest.Destination) == "" && strings.TrimSpace(src.Destination) != "" {
			dest.Destination = src.Destination
			dest.UpdatedAt = now
		}
	}
	moved := make([]int, 0, len(ids))
	for i := range s.db.Issues {
		if !ids[s.db.Issues[i].ID] {
			continue
		}
		s.db.Issues[i].ProjectID = &pid
		s.db.Issues[i].UpdatedAt = now
		moved = append(moved, s.db.Issues[i].ID)
	}
	sort.Ints(moved)
	if err := s.saveLocked(); err != nil {
		return MoveResult{}, err
	}
	return MoveResult{Moved: moved, Project: s.projectViewLocked(*s.findProjectLocked(pid))}, nil
}

func (s *Store) DeleteProject(id int) (DeleteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.findProjectLocked(id) == nil {
		return DeleteResult{}, fmt.Errorf("%w: project %d", ErrNotFound, id)
	}
	drop := map[int]bool{}
	for _, issue := range s.issuesForProjectLocked(id) {
		for _, did := range s.descendantsLocked(issue.ID) {
			drop[did] = true
		}
	}
	keptIssues := make([]model.Issue, 0, len(s.db.Issues)-len(drop))
	deleted := make([]int, 0, len(drop))
	for _, issue := range s.db.Issues {
		if drop[issue.ID] {
			deleted = append(deleted, issue.ID)
			continue
		}
		keptIssues = append(keptIssues, issue)
	}
	sort.Ints(deleted)
	for i := range keptIssues {
		keptIssues[i].BlockedBy = stripIDs(keptIssues[i].BlockedBy, drop)
		keptIssues[i].LinkedMaps = stripIDs(keptIssues[i].LinkedMaps, drop)
	}
	s.db.Issues = keptIssues
	keptProjects := s.db.Projects[:0]
	for _, p := range s.db.Projects {
		if p.ID != id {
			keptProjects = append(keptProjects, p)
		}
	}
	s.db.Projects = keptProjects
	if err := s.saveLocked(); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{Deleted: deleted}, nil
}

func (s *Store) ReadyForSpec(mapID int) (model.IssueView, error) {
	return s.setMapLifecycle(mapID, model.MapLifecycleReadyForSpec)
}

func (s *Store) ClearRoute(mapID int) (model.IssueView, error) {
	return s.setMapLifecycle(mapID, model.MapLifecycleCleared)
}

func (s *Store) setMapLifecycle(mapID int, life string) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, issue, ok := s.findIndexLocked(mapID)
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	if !model.IsMap(issue) {
		return model.IssueView{}, fmt.Errorf("%w: issue %d is not a map", ErrInvalid, mapID)
	}
	issue.Lifecycle = life
	issue.Kind = model.KindDecisionMap
	issue.UpdatedAt = time.Now().UTC()
	s.db.Issues[idx] = issue
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(issue), nil
}

func (s *Store) CreateSpec(mapID int) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, src, ok := s.findIndexLocked(mapID)
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	if !model.IsMap(src) {
		return model.IssueView{}, fmt.Errorf("%w: issue %d is not a map", ErrInvalid, mapID)
	}
	if src.Lifecycle != model.MapLifecycleReadyForSpec {
		return model.IssueView{}, fmt.Errorf("%w: map must be ready_for_spec", ErrInvalid)
	}
	return s.createSpecLocked(src)
}

// AdvanceToSpec marks a Decision Map ready_for_spec and creates its draft
// Spec in one step, so "make the spec" is a single action. It composes
// ReadyForSpec and CreateSpec: the map may be active or already
// ready_for_spec, and an existing spec is the same error as CreateSpec.
func (s *Store) AdvanceToSpec(mapID int) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, issue, ok := s.findIndexLocked(mapID)
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	if !model.IsMap(issue) {
		return model.IssueView{}, fmt.Errorf("%w: issue %d is not a map", ErrInvalid, mapID)
	}
	issue.Lifecycle = model.MapLifecycleReadyForSpec
	issue.Kind = model.KindDecisionMap
	issue.UpdatedAt = time.Now().UTC()
	s.db.Issues[idx] = issue
	return s.createSpecLocked(issue)
}

func (s *Store) createSpecLocked(src model.Issue) (model.IssueView, error) {
	if existing, found := s.derivedLocked(model.KindSpec, src.ID); found {
		return model.IssueView{}, fmt.Errorf("%w: spec already exists (%s)", ErrInvalid, existing.Identifier)
	}
	dest := model.ExtractDestination(src.Body)
	if src.ProjectID != nil {
		if p := s.findProjectLocked(*src.ProjectID); p != nil && strings.TrimSpace(p.Destination) != "" {
			dest = p.Destination
		}
	}
	now := time.Now().UTC()
	id := s.db.NextID
	s.db.NextID++
	from := src.ID
	issue := model.Issue{
		ID:                    id,
		Identifier:            fmt.Sprintf("%s-%d", s.db.Prefix, id),
		Title:                 "Spec: " + src.Title,
		Body:                  model.SpecSkeleton(dest),
		State:                 model.StateOpen,
		Labels:                []string{},
		BlockedBy:             []int{},
		LinkedMaps:            []int{},
		Project:               src.Project,
		Kind:                  model.KindSpec,
		ProjectID:             cloneInt(src.ProjectID),
		Lifecycle:             model.SpecLifecycleDraft,
		DerivedFromArtifactID: &from,
		CreatedAt:             now,
		UpdatedAt:             now,
		Comments:              []model.Comment{},
	}
	s.db.Issues = append(s.db.Issues, issue)
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(issue), nil
}

func (s *Store) ApproveSpec(specID int) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, issue, ok := s.findIndexLocked(specID)
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	if !model.IsSpec(issue) {
		return model.IssueView{}, fmt.Errorf("%w: issue %d is not a spec", ErrInvalid, specID)
	}
	if issue.Lifecycle != model.SpecLifecycleDraft && issue.Lifecycle != model.SpecLifecycleApproved {
		return model.IssueView{}, fmt.Errorf("%w: spec cannot be approved from %s", ErrInvalid, issue.Lifecycle)
	}
	issue.Lifecycle = model.SpecLifecycleApproved
	issue.UpdatedAt = time.Now().UTC()
	s.db.Issues[idx] = issue
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(issue), nil
}

func (s *Store) CreatePlan(specID int) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, src, ok := s.findIndexLocked(specID)
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	if !model.IsSpec(src) {
		return model.IssueView{}, fmt.Errorf("%w: issue %d is not a spec", ErrInvalid, specID)
	}
	if src.Lifecycle != model.SpecLifecycleApproved {
		return model.IssueView{}, fmt.Errorf("%w: spec must be approved", ErrInvalid)
	}
	if existing, found := s.derivedLocked(model.KindPlan, specID); found {
		return model.IssueView{}, fmt.Errorf("%w: plan already exists (%s)", ErrInvalid, existing.Identifier)
	}
	now := time.Now().UTC()
	id := s.db.NextID
	s.db.NextID++
	from := src.ID
	title := strings.TrimPrefix(src.Title, "Spec: ")
	issue := model.Issue{
		ID:                    id,
		Identifier:            fmt.Sprintf("%s-%d", s.db.Prefix, id),
		Title:                 "Plan: " + title,
		Body:                  model.PlanSkeleton(),
		State:                 model.StateOpen,
		Labels:                []string{},
		BlockedBy:             []int{},
		LinkedMaps:            []int{},
		Project:               src.Project,
		Kind:                  model.KindPlan,
		ProjectID:             cloneInt(src.ProjectID),
		Lifecycle:             model.PlanLifecycleDraft,
		DerivedFromArtifactID: &from,
		CreatedAt:             now,
		UpdatedAt:             now,
		Comments:              []model.Comment{},
	}
	s.db.Issues = append(s.db.Issues, issue)
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(issue), nil
}

func (s *Store) ActivatePlan(planID int) (model.IssueView, error) {
	return s.setPlanLifecycle(planID, model.PlanLifecycleActive, model.PlanLifecycleDraft, model.PlanLifecycleActive)
}

func (s *Store) DeliverPlan(planID int) (model.IssueView, error) {
	return s.setPlanLifecycle(planID, model.PlanLifecycleDelivered, model.PlanLifecycleActive, model.PlanLifecycleDelivered)
}

func (s *Store) setPlanLifecycle(planID int, life string, allowed ...string) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, issue, ok := s.findIndexLocked(planID)
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	if !model.IsPlan(issue) {
		return model.IssueView{}, fmt.Errorf("%w: issue %d is not a plan", ErrInvalid, planID)
	}
	okLife := false
	for _, a := range allowed {
		if issue.Lifecycle == a {
			okLife = true
			break
		}
	}
	if !okLife {
		return model.IssueView{}, fmt.Errorf("%w: plan cannot move to %s from %s", ErrInvalid, life, issue.Lifecycle)
	}
	issue.Lifecycle = life
	issue.UpdatedAt = time.Now().UTC()
	s.db.Issues[idx] = issue
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(issue), nil
}

func (s *Store) derivedLocked(kind string, fromID int) (model.Issue, bool) {
	for _, issue := range s.db.Issues {
		if issue.Kind != kind || issue.DerivedFromArtifactID == nil || *issue.DerivedFromArtifactID != fromID {
			continue
		}
		if kind == model.KindSpec && issue.Lifecycle == model.SpecLifecycleSuperseded {
			continue
		}
		return issue, true
	}
	return model.Issue{}, false
}

func cloneInt(v *int) *int {
	if v == nil {
		return nil
	}
	n := *v
	return &n
}
