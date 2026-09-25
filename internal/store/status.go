package store

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/johnspence0212/NonLinear/internal/model"
)

// ProjectStatus returns a compact big-picture snapshot of a Project.
// Pass id, or query (identifier like P-6, numeric id, or title).
func (s *Store) ProjectStatus(id *int, query string) (model.ProjectStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.lookupProjectLocked(id, query)
	if err != nil {
		return model.ProjectStatus{}, err
	}
	return s.projectStatusLocked(*p), nil
}

func (s *Store) projectStatusLocked(p model.Project) model.ProjectStatus {
	return model.BuildProjectStatus(p, s.issuesForProjectLocked(p.ID), s.byIDLocked())
}

func (s *Store) lookupProjectLocked(id *int, query string) (*model.Project, error) {
	if id != nil {
		p := s.findProjectLocked(*id)
		if p == nil {
			return nil, fmt.Errorf("%w: project %d", ErrNotFound, *id)
		}
		return p, nil
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("%w: id or query is required", ErrInvalid)
	}
	if n, ok := parseProjectRef(q); ok {
		if p := s.findProjectLocked(n); p != nil {
			return p, nil
		}
	}
	lower := strings.ToLower(q)
	var exact, partial []*model.Project
	for i := range s.db.Projects {
		p := &s.db.Projects[i]
		ident := strings.ToLower(p.Identifier)
		title := strings.ToLower(p.Title)
		if ident == lower || title == lower {
			exact = append(exact, p)
			continue
		}
		if strings.Contains(ident, lower) || strings.Contains(title, lower) {
			partial = append(partial, p)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		return nil, fmt.Errorf("%w: query %q matches %s", ErrInvalid, q, formatProjectMatches(exact))
	}
	if len(partial) == 1 {
		return partial[0], nil
	}
	if len(partial) > 1 {
		return nil, fmt.Errorf("%w: query %q matches %s", ErrInvalid, q, formatProjectMatches(partial))
	}
	return nil, fmt.Errorf("%w: project %q", ErrNotFound, q)
}

func parseProjectRef(q string) (int, bool) {
	if n, err := strconv.Atoi(q); err == nil && n > 0 {
		return n, true
	}
	upper := strings.ToUpper(q)
	if strings.HasPrefix(upper, "P-") {
		if n, err := strconv.Atoi(upper[2:]); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

func formatProjectMatches(projects []*model.Project) string {
	parts := make([]string, 0, len(projects))
	for _, p := range projects {
		parts = append(parts, fmt.Sprintf("%s %s", p.Identifier, p.Title))
	}
	return strings.Join(parts, ", ")
}
