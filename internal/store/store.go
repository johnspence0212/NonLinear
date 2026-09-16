package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/johnspence0212/NonLinear/internal/model"
)

var (
	ErrNotFound     = errors.New("issue not found")
	ErrInvalid      = errors.New("invalid request")
	ErrParentCycle  = errors.New("parent would create a cycle")
	ErrSelfRelation = errors.New("issue cannot reference itself")
)

type Store struct {
	path string
	mu   sync.Mutex
	db   model.DB
}

type ListFilter struct {
	State        string
	Labels       []string
	ParentID     *int
	Assignee     *string // nil = any; pointer to "" or "unassigned" = unassigned
	Project      string
	Query        string
	FrontierOnly bool
}

type CreateIssue struct {
	Title    string
	Body     string
	Labels   []string
	ParentID *int
	Project  string
	Assignee *string
}

type UpdateIssue struct {
	Title    *string
	Body     *string
	Labels   *[]string
	State    *string
	Assignee *string // pointer to "" unassigns
	ParentID **int   // pointer to nil pointer clears parent
	Project  *string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	s := &Store{path: path}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		s.db = model.DB{NextID: 1, Prefix: model.DefaultPrefix, Issues: []model.Issue{}}
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
		return s, nil
	} else if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		s.db = model.DB{NextID: 1, Prefix: model.DefaultPrefix, Issues: []model.Issue{}}
		return s, s.saveLocked()
	}
	if err := json.Unmarshal(raw, &s.db); err != nil {
		return nil, fmt.Errorf("parse db: %w", err)
	}
	if s.db.Prefix == "" {
		s.db.Prefix = model.DefaultPrefix
	}
	if s.db.NextID < 1 {
		s.db.NextID = 1
	}
	if s.db.Issues == nil {
		s.db.Issues = []model.Issue{}
	}
	return s, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Labels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	out := append([]string{}, model.SeedLabels...)
	for _, l := range model.SeedLabels {
		seen[l] = true
	}
	for _, issue := range s.db.Issues {
		for _, l := range issue.Labels {
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	sort.Strings(out[len(model.SeedLabels):])
	return out
}

func (s *Store) List(filter ListFilter) []model.IssueView {
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := s.byIDLocked()
	out := []model.IssueView{}
	for _, issue := range s.db.Issues {
		if !match(issue, filter, byID) {
			continue
		}
		out = append(out, model.View(issue, byID))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Store) Get(id int) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := s.byIDLocked()
	issue, ok := byID[id]
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	return model.View(issue, byID), nil
}

func (s *Store) Create(in CreateIssue) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return model.IssueView{}, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if in.ParentID != nil {
		if _, ok := s.findLocked(*in.ParentID); !ok {
			return model.IssueView{}, fmt.Errorf("%w: parent %d", ErrNotFound, *in.ParentID)
		}
	}
	now := time.Now().UTC()
	id := s.db.NextID
	s.db.NextID++
	project := strings.TrimSpace(in.Project)
	if project == "" {
		project = model.DefaultProject
	}
	assignee := normalizeAssignee(in.Assignee)
	issue := model.Issue{
		ID:         id,
		Identifier: fmt.Sprintf("%s-%d", s.db.Prefix, id),
		Title:      title,
		Body:       in.Body,
		State:      model.StateOpen,
		Labels:     uniqueStrings(in.Labels),
		Assignee:   assignee,
		ParentID:   in.ParentID,
		BlockedBy:  []int{},
		Project:    project,
		CreatedAt:  now,
		UpdatedAt:  now,
		Comments:   []model.Comment{},
	}
	s.db.Issues = append(s.db.Issues, issue)
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return model.View(issue, s.byIDLocked()), nil
}

func (s *Store) Update(id int, in UpdateIssue) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, issue, ok := s.findIndexLocked(id)
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			return model.IssueView{}, fmt.Errorf("%w: title is required", ErrInvalid)
		}
		issue.Title = title
	}
	if in.Body != nil {
		issue.Body = *in.Body
	}
	if in.Labels != nil {
		issue.Labels = uniqueStrings(*in.Labels)
	}
	if in.State != nil {
		state := strings.TrimSpace(*in.State)
		if state != model.StateOpen && state != model.StateClosed {
			return model.IssueView{}, fmt.Errorf("%w: state must be open or closed", ErrInvalid)
		}
		issue.State = state
	}
	if in.Assignee != nil {
		issue.Assignee = normalizeAssignee(in.Assignee)
	}
	if in.ParentID != nil {
		parent := *in.ParentID
		if parent != nil {
			if *parent == id {
				return model.IssueView{}, ErrSelfRelation
			}
			if _, ok := s.findLocked(*parent); !ok {
				return model.IssueView{}, fmt.Errorf("%w: parent %d", ErrNotFound, *parent)
			}
			if s.wouldCycleLocked(id, *parent) {
				return model.IssueView{}, ErrParentCycle
			}
		}
		issue.ParentID = parent
	}
	if in.Project != nil {
		project := strings.TrimSpace(*in.Project)
		if project == "" {
			project = model.DefaultProject
		}
		issue.Project = project
	}
	issue.UpdatedAt = time.Now().UTC()
	s.db.Issues[idx] = issue
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return model.View(issue, s.byIDLocked()), nil
}

func (s *Store) SetBlockedBy(id int, blockerIDs []int) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, issue, ok := s.findIndexLocked(id)
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	seen := map[int]bool{}
	clean := []int{}
	for _, bid := range blockerIDs {
		if bid == id {
			return model.IssueView{}, ErrSelfRelation
		}
		if seen[bid] {
			continue
		}
		if _, ok := s.findLocked(bid); !ok {
			return model.IssueView{}, fmt.Errorf("%w: blocker %d", ErrNotFound, bid)
		}
		seen[bid] = true
		clean = append(clean, bid)
	}
	issue.BlockedBy = clean
	issue.UpdatedAt = time.Now().UTC()
	s.db.Issues[idx] = issue
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return model.View(issue, s.byIDLocked()), nil
}

func (s *Store) AddComment(id int, author, body string) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, issue, ok := s.findIndexLocked(id)
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return model.IssueView{}, fmt.Errorf("%w: comment body is required", ErrInvalid)
	}
	author = strings.TrimSpace(author)
	if author == "" {
		author = "cursor"
	}
	issue.Comments = append(issue.Comments, model.Comment{
		ID:        newID(),
		Author:    author,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	})
	issue.UpdatedAt = time.Now().UTC()
	s.db.Issues[idx] = issue
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return model.View(issue, s.byIDLocked()), nil
}

func (s *Store) Claim(id int, assignee string) (model.IssueView, error) {
	assignee = strings.TrimSpace(assignee)
	if assignee == "" {
		assignee = "cursor"
	}
	return s.Update(id, UpdateIssue{Assignee: &assignee})
}

func (s *Store) Resolve(id int, author, answer string) (model.IssueView, error) {
	if _, err := s.AddComment(id, author, answer); err != nil {
		return model.IssueView{}, err
	}
	closed := model.StateClosed
	return s.Update(id, UpdateIssue{State: &closed})
}

func (s *Store) Frontier(parentID *int) []model.IssueView {
	return s.List(ListFilter{State: model.StateOpen, ParentID: parentID, FrontierOnly: true})
}

func match(issue model.Issue, filter ListFilter, byID map[int]model.Issue) bool {
	if filter.State != "" && issue.State != filter.State {
		return false
	}
	for _, label := range filter.Labels {
		if !model.HasLabel(issue, label) {
			return false
		}
	}
	if filter.ParentID != nil {
		if issue.ParentID == nil || *issue.ParentID != *filter.ParentID {
			return false
		}
	}
	if filter.Assignee != nil {
		want := strings.TrimSpace(*filter.Assignee)
		got := model.AssigneeValue(issue)
		if want == "" || strings.EqualFold(want, "unassigned") {
			if got != "" {
				return false
			}
		} else if !strings.EqualFold(got, want) {
			return false
		}
	}
	if filter.Project != "" && !strings.EqualFold(issue.Project, filter.Project) {
		return false
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		q = strings.ToLower(q)
		blob := strings.ToLower(issue.Identifier + " " + issue.Title + " " + issue.Body)
		if !strings.Contains(blob, q) {
			return false
		}
	}
	if filter.FrontierOnly && !model.IsFrontier(issue, byID) {
		return false
	}
	return true
}

func (s *Store) byIDLocked() map[int]model.Issue {
	out := make(map[int]model.Issue, len(s.db.Issues))
	for _, issue := range s.db.Issues {
		out[issue.ID] = issue
	}
	return out
}

func (s *Store) findLocked(id int) (model.Issue, bool) {
	for _, issue := range s.db.Issues {
		if issue.ID == id {
			return issue, true
		}
	}
	return model.Issue{}, false
}

func (s *Store) findIndexLocked(id int) (int, model.Issue, bool) {
	for i, issue := range s.db.Issues {
		if issue.ID == id {
			return i, issue, true
		}
	}
	return -1, model.Issue{}, false
}

func (s *Store) wouldCycleLocked(id, parent int) bool {
	seen := map[int]bool{id: true}
	cur := parent
	for {
		if seen[cur] {
			return true
		}
		seen[cur] = true
		issue, ok := s.findLocked(cur)
		if !ok || issue.ParentID == nil {
			return false
		}
		cur = *issue.ParentID
	}
}

func (s *Store) saveLocked() error {
	if s.db.Issues == nil {
		s.db.Issues = []model.Issue{}
	}
	raw, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "db-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, s.path)
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func normalizeAssignee(in *string) *string {
	if in == nil {
		return nil
	}
	v := strings.TrimSpace(*in)
	if v == "" || strings.EqualFold(v, "unassigned") {
		return nil
	}
	return &v
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
