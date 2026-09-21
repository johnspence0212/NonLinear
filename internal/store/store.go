package store

import (
	"crypto/rand"
	"encoding/hex"
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
	path  string
	mu    sync.Mutex
	db    model.DB
	extra leftover
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
	Title       string
	Body        string
	Labels      []string
	ParentID    *int
	LinkedMapID *int
	Project     string
	ProjectID   *int
	Assignee    *string
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
		s.db = emptyDB()
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
		s.db = emptyDB()
		return s, s.saveLocked()
	}
	db, extra, err := DecodeDocument(raw)
	if err != nil {
		return nil, err
	}
	s.db = db
	s.extra = extra
	return s, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.db.Issues)
}

type DeleteResult struct {
	Deleted []int `json:"deleted"`
}

// Delete removes an issue and every descendant. Remaining issues drop
// any blocked-by edges that pointed at the removed ids.
func (s *Store) Delete(id int) (DeleteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.findLocked(id); !ok {
		return DeleteResult{}, ErrNotFound
	}
	drop := map[int]bool{}
	for _, did := range s.descendantsLocked(id) {
		drop[did] = true
	}
	kept := make([]model.Issue, 0, len(s.db.Issues)-len(drop))
	deleted := make([]int, 0, len(drop))
	for _, issue := range s.db.Issues {
		if drop[issue.ID] {
			deleted = append(deleted, issue.ID)
			continue
		}
		kept = append(kept, issue)
	}
	sort.Ints(deleted)
	for i := range kept {
		kept[i].BlockedBy = stripIDs(kept[i].BlockedBy, drop)
		kept[i].LinkedMaps = stripIDs(kept[i].LinkedMaps, drop)
	}
	s.db.Issues = kept
	if err := s.saveLocked(); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{Deleted: deleted}, nil
}

// Wipe empties the tracker and resets ids so the next issue is NL-1.
func (s *Store) Wipe() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.db.Issues)
	s.db.Issues = []model.Issue{}
	s.db.Labels = []string{}
	s.db.Projects = []model.Project{}
	s.db.NextID = 1
	s.db.NextProjectID = 1
	return n, s.saveLocked()
}

// ExportMap returns a portable bundle for a wayfinder map and every descendant.
// Blocked-by edges that point outside the map are dropped.
func (s *Store) ExportMap(id int) (model.MapBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, ok := s.findLocked(id)
	if !ok {
		return model.MapBundle{}, ErrNotFound
	}
	if !model.IsMap(root) {
		return model.MapBundle{}, fmt.Errorf("%w: issue %d is not a map", ErrInvalid, id)
	}
	keep := map[int]bool{}
	for _, did := range s.descendantsLocked(id) {
		keep[did] = true
	}
	issues := make([]model.Issue, 0, len(keep))
	for _, issue := range s.db.Issues {
		if !keep[issue.ID] {
			continue
		}
		out := model.CloneIssue(issue)
		if issue.ID == id {
			out.ParentID = nil
		} else if out.ParentID != nil && !keep[*out.ParentID] {
			rootID := id
			out.ParentID = &rootID
		}
		out.BlockedBy = keepIDs(out.BlockedBy, keep)
		out.LinkedMaps = keepIDs(out.LinkedMaps, keep)
		issues = append(issues, out)
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	return model.MapBundle{
		Kind:       model.MapBundleKind,
		Version:    model.MapBundleVersion,
		ExportedAt: time.Now().UTC(),
		RootID:     id,
		Issues:     issues,
	}, nil
}

type ImportResult struct {
	Map     model.IssueView `json:"map"`
	Created []int           `json:"created"`
}

// ImportMap copies a map bundle into this tracker with new ids. Existing
// issues are left alone. Blocked-by and parent edges remapped within the
// bundle; the imported map is always top-level.
func (s *Store) ImportMap(bundle model.MapBundle) (ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	issues, rootOld, err := validateMapBundle(bundle)
	if err != nil {
		return ImportResult{}, err
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	idMap := make(map[int]int, len(issues))
	for _, old := range issues {
		idMap[old.ID] = s.db.NextID
		s.db.NextID++
	}
	imported := make([]model.Issue, 0, len(issues))
	created := make([]int, 0, len(issues))
	rootNew := idMap[rootOld]
	for _, old := range issues {
		issue := model.CloneIssue(old)
		issue.ID = idMap[old.ID]
		issue.Identifier = fmt.Sprintf("%s-%d", s.db.Prefix, issue.ID)
		if old.ID == rootOld {
			issue.ParentID = nil
			if !model.HasLabel(issue, "wayfinder:map") {
				issue.Labels = append([]string{"wayfinder:map"}, issue.Labels...)
			}
		} else if issue.ParentID != nil {
			if nid, ok := idMap[*issue.ParentID]; ok && nid != issue.ID {
				issue.ParentID = &nid
			} else {
				issue.ParentID = &rootNew
			}
		} else {
			issue.ParentID = &rootNew
		}
		blocked := make([]int, 0, len(issue.BlockedBy))
		seen := map[int]bool{}
		for _, bid := range issue.BlockedBy {
			nid, ok := idMap[bid]
			if !ok || nid == issue.ID || seen[nid] {
				continue
			}
			seen[nid] = true
			blocked = append(blocked, nid)
		}
		issue.BlockedBy = blocked
		linked := make([]int, 0, len(issue.LinkedMaps))
		seenLinked := map[int]bool{}
		for _, lid := range issue.LinkedMaps {
			nid, ok := idMap[lid]
			if !ok || nid == issue.ID || seenLinked[nid] {
				continue
			}
			seenLinked[nid] = true
			linked = append(linked, nid)
		}
		issue.LinkedMaps = linked
		comments := make([]model.Comment, 0, len(issue.Comments))
		for _, c := range issue.Comments {
			c.ID = newID()
			if strings.TrimSpace(c.Author) == "" {
				c.Author = "cursor"
			}
			comments = append(comments, c)
		}
		issue.Comments = comments
		if issue.Project == "" {
			issue.Project = model.DefaultProject
		}
		if issue.State != model.StateOpen && issue.State != model.StateClosed {
			issue.State = model.StateOpen
		}
		imported = append(imported, issue)
		created = append(created, issue.ID)
	}
	if parentCycle(imported) {
		return ImportResult{}, ErrParentCycle
	}
	s.db.Issues = append(s.db.Issues, imported...)
	NormalizeDocument(&s.db)
	if err := s.saveLocked(); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{
		Map:     s.viewLocked(s.byIDLocked()[rootNew]),
		Created: created,
	}, nil
}

func (s *Store) descendantsLocked(id int) []int {
	kids := map[int][]int{}
	for _, issue := range s.db.Issues {
		if issue.ParentID != nil {
			kids[*issue.ParentID] = append(kids[*issue.ParentID], issue.ID)
		}
	}
	seen := map[int]bool{}
	out := []int{}
	var walk func(int)
	walk = func(cur int) {
		if seen[cur] {
			return
		}
		seen[cur] = true
		out = append(out, cur)
		for _, child := range kids[cur] {
			walk(child)
		}
	}
	walk(id)
	return out
}

func stripIDs(ids []int, drop map[int]bool) []int {
	out := []int{}
	for _, id := range ids {
		if !drop[id] {
			out = append(out, id)
		}
	}
	return out
}

func keepIDs(ids []int, keep map[int]bool) []int {
	out := []int{}
	for _, id := range ids {
		if keep[id] {
			out = append(out, id)
		}
	}
	return out
}

func validateMapBundle(bundle model.MapBundle) ([]model.Issue, int, error) {
	if bundle.Kind != model.MapBundleKind {
		return nil, 0, fmt.Errorf("%w: not a nonlinear map bundle", ErrInvalid)
	}
	if bundle.Version != 0 && bundle.Version != model.MapBundleVersion {
		return nil, 0, fmt.Errorf("%w: unsupported map bundle version %d", ErrInvalid, bundle.Version)
	}
	if len(bundle.Issues) == 0 {
		return nil, 0, fmt.Errorf("%w: map bundle has no issues", ErrInvalid)
	}
	seen := map[int]bool{}
	issues := make([]model.Issue, 0, len(bundle.Issues))
	maps := []int{}
	for _, issue := range bundle.Issues {
		if issue.ID < 1 {
			return nil, 0, fmt.Errorf("%w: issue id is required", ErrInvalid)
		}
		if seen[issue.ID] {
			return nil, 0, fmt.Errorf("%w: duplicate issue id %d", ErrInvalid, issue.ID)
		}
		if strings.TrimSpace(issue.Title) == "" {
			return nil, 0, fmt.Errorf("%w: title is required", ErrInvalid)
		}
		seen[issue.ID] = true
		cloned := model.CloneIssue(issue)
		if model.IsMap(cloned) {
			maps = append(maps, cloned.ID)
		}
		issues = append(issues, cloned)
	}
	root := bundle.RootID
	if root == 0 {
		if len(maps) == 1 {
			root = maps[0]
		} else {
			return nil, 0, fmt.Errorf("%w: rootId is required", ErrInvalid)
		}
	}
	if !seen[root] {
		return nil, 0, fmt.Errorf("%w: root %d", ErrNotFound, root)
	}
	return issues, root, nil
}

func parentCycle(issues []model.Issue) bool {
	byID := make(map[int]model.Issue, len(issues))
	for _, issue := range issues {
		byID[issue.ID] = issue
	}
	for _, issue := range issues {
		seen := map[int]bool{issue.ID: true}
		cur := issue.ParentID
		for cur != nil {
			if seen[*cur] {
				return true
			}
			seen[*cur] = true
			next, ok := byID[*cur]
			if !ok {
				break
			}
			cur = next.ParentID
		}
	}
	return false
}

func (s *Store) Labels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.labelsLocked()
}

type LabelResult struct {
	Label   string   `json:"label"`
	Created bool     `json:"created"`
	Labels  []string `json:"labels"`
}

// CreateLabel records a tag in the tracker catalog so it appears in the
// label list even when no issue uses it yet. Idempotent.
func (s *Store) CreateLabel(name string) (LabelResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	label, err := normalizeLabel(name)
	if err != nil {
		return LabelResult{}, err
	}
	created := s.ensureLabelLocked(label)
	if created {
		if err := s.saveLocked(); err != nil {
			return LabelResult{}, err
		}
	}
	return LabelResult{Label: label, Created: created, Labels: s.labelsLocked()}, nil
}

// AddLabel appends a tag to an issue without replacing existing labels.
// The tag is also recorded in the catalog.
func (s *Store) AddLabel(id int, name string) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	label, err := normalizeLabel(name)
	if err != nil {
		return model.IssueView{}, err
	}
	idx, issue, ok := s.findIndexLocked(id)
	if !ok {
		return model.IssueView{}, ErrNotFound
	}
	s.ensureLabelLocked(label)
	if !model.HasLabel(issue, label) {
		issue.Labels = append(append([]string{}, issue.Labels...), label)
		issue.UpdatedAt = time.Now().UTC()
		s.db.Issues[idx] = issue
	}
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(issue), nil
}

func (s *Store) labelsLocked() []string {
	seen := map[string]bool{}
	out := append([]string{}, model.SeedLabels...)
	for _, l := range model.SeedLabels {
		seen[l] = true
	}
	for _, l := range s.db.Labels {
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	for _, issue := range s.db.Issues {
		for _, l := range issue.Labels {
			if l == "" || seen[l] {
				continue
			}
			seen[l] = true
			out = append(out, l)
		}
	}
	sort.Strings(out[len(model.SeedLabels):])
	return out
}

func (s *Store) ensureLabelLocked(label string) bool {
	for _, l := range model.SeedLabels {
		if l == label {
			return false
		}
	}
	for _, l := range s.db.Labels {
		if l == label {
			return false
		}
	}
	s.db.Labels = append(s.db.Labels, label)
	sort.Strings(s.db.Labels)
	return true
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
		out = append(out, s.viewLocked(issue))
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
	return s.viewLocked(issue), nil
}

func (s *Store) Create(in CreateIssue) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return model.IssueView{}, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if in.ParentID != nil {
		if in.LinkedMapID != nil {
			return model.IssueView{}, fmt.Errorf("%w: linked maps cannot have a parent ticket", ErrInvalid)
		}
		if _, ok := s.findLocked(*in.ParentID); !ok {
			return model.IssueView{}, fmt.Errorf("%w: parent %d", ErrNotFound, *in.ParentID)
		}
	}
	if in.ProjectID != nil && s.findProjectLocked(*in.ProjectID) == nil {
		return model.IssueView{}, fmt.Errorf("%w: project %d", ErrNotFound, *in.ProjectID)
	}
	labels := uniqueStrings(in.Labels)
	var linkedTarget *model.Issue
	if in.LinkedMapID != nil {
		target, ok := s.findLocked(*in.LinkedMapID)
		if !ok {
			return model.IssueView{}, fmt.Errorf("%w: map %d", ErrNotFound, *in.LinkedMapID)
		}
		if !model.IsMap(target) {
			return model.IssueView{}, fmt.Errorf("%w: issue %d is not a map", ErrInvalid, *in.LinkedMapID)
		}
		linkedTarget = &target
		if !model.HasLabel(model.Issue{Labels: labels}, "wayfinder:map") {
			labels = append([]string{"wayfinder:map"}, labels...)
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
		Labels:     labels,
		Assignee:   assignee,
		ParentID:   in.ParentID,
		BlockedBy:  []int{},
		LinkedMaps: []int{},
		Project:    project,
		CreatedAt:  now,
		UpdatedAt:  now,
		Comments:   []model.Comment{},
	}
	issue.ProjectID = s.createProjectIDLocked(in, linkedTarget)
	s.db.Issues = append(s.db.Issues, issue)
	s.ensureMapProjectLocked(len(s.db.Issues) - 1)
	if in.LinkedMapID != nil {
		if err := s.setLinkedMapsLocked(id, []int{*in.LinkedMapID}); err != nil {
			return model.IssueView{}, err
		}
	}
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(s.byIDLocked()[id]), nil
}

// createProjectIDLocked picks the Project a new issue belongs to.
// Explicit projectId wins. Otherwise inherit from parent, then from a
// linked source map. Maps with none wrap into an implicit Project later.
func (s *Store) createProjectIDLocked(in CreateIssue, linkedTarget *model.Issue) *int {
	if in.ProjectID != nil {
		pid := *in.ProjectID
		return &pid
	}
	if in.ParentID != nil {
		if parent, ok := s.findLocked(*in.ParentID); ok && parent.ProjectID != nil {
			pid := *parent.ProjectID
			return &pid
		}
	}
	if linkedTarget != nil && linkedTarget.ProjectID != nil {
		pid := *linkedTarget.ProjectID
		return &pid
	}
	return nil
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
	s.ensureMapProjectLocked(idx)
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(issue), nil
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
	return s.viewLocked(issue), nil
}

func (s *Store) SetLinkedMaps(id int, mapIDs []int) (model.IssueView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.setLinkedMapsLocked(id, mapIDs); err != nil {
		return model.IssueView{}, err
	}
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(s.byIDLocked()[id]), nil
}

func (s *Store) setLinkedMapsLocked(id int, mapIDs []int) error {
	idx, issue, ok := s.findIndexLocked(id)
	if !ok {
		return ErrNotFound
	}
	if !model.IsMap(issue) {
		return fmt.Errorf("%w: issue %d is not a map", ErrInvalid, id)
	}
	seen := map[int]bool{}
	clean := []int{}
	for _, mid := range mapIDs {
		if mid == id {
			return ErrSelfRelation
		}
		if seen[mid] {
			continue
		}
		other, ok := s.findLocked(mid)
		if !ok {
			return fmt.Errorf("%w: map %d", ErrNotFound, mid)
		}
		if !model.IsMap(other) {
			return fmt.Errorf("%w: issue %d is not a map", ErrInvalid, mid)
		}
		seen[mid] = true
		clean = append(clean, mid)
	}
	issue.LinkedMaps = clean
	issue.UpdatedAt = time.Now().UTC()
	s.db.Issues[idx] = issue
	now := issue.UpdatedAt
	for i := range s.db.Issues {
		other := s.db.Issues[i]
		if other.ID == id {
			continue
		}
		has := false
		for _, lid := range other.LinkedMaps {
			if lid == id {
				has = true
				break
			}
		}
		want := seen[other.ID]
		if has == want {
			continue
		}
		if want {
			other.LinkedMaps = append(append([]int{}, other.LinkedMaps...), id)
		} else {
			other.LinkedMaps = stripIDs(other.LinkedMaps, map[int]bool{id: true})
		}
		other.UpdatedAt = now
		s.db.Issues[i] = other
	}
	return nil
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
	return s.viewLocked(issue), nil
}

func (s *Store) UpdateComment(id int, commentID, body string) (model.IssueView, error) {
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
	commentID = strings.TrimSpace(commentID)
	found := false
	now := time.Now().UTC()
	for i, c := range issue.Comments {
		if c.ID != commentID {
			continue
		}
		c.Body = body
		c.UpdatedAt = &now
		issue.Comments[i] = c
		found = true
		break
	}
	if !found {
		return model.IssueView{}, fmt.Errorf("%w: comment", ErrNotFound)
	}
	issue.UpdatedAt = now
	s.db.Issues[idx] = issue
	if err := s.saveLocked(); err != nil {
		return model.IssueView{}, err
	}
	return s.viewLocked(issue), nil
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
	raw, err := EncodeDocument(s.db, s.extra)
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

func normalizeLabel(name string) (string, error) {
	s := strings.TrimSpace(name)
	s = strings.TrimLeft(s, "#")
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%w: label is required", ErrInvalid)
	}
	if strings.ContainsAny(s, ", \t\n\r") {
		return "", fmt.Errorf("%w: label cannot contain spaces or commas", ErrInvalid)
	}
	return s, nil
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
